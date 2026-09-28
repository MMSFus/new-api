package sessionrecorder

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Store is the remote object store used by the consumer. The interface has
// no delete or copy operation on purpose.
type Store interface {
	Head(ctx context.Context, key string) (ObjectMetadata, error)
	PutFile(ctx context.Context, key, file string, size int64, sha string) error
	PutBytes(ctx context.Context, key string, data []byte, metadata map[string]string) error
	SetBusy(busy bool)
}

// CycleResult summarises one consumer cycle.
type CycleResult struct {
	State        string
	BatchID      string
	ScannedFiles int
	OrphanMedia  int
	Issues       int
}

// StatusReport keeps the standalone field order; new-api only appends
// fields after the original ones.
type StatusReport struct {
	SetupCompleted      bool            `json:"setup_completed"`
	UploadEnabled       bool            `json:"upload_enabled"`
	LocalCleanupEnabled bool            `json:"local_cleanup_enabled"`
	CredentialState     CredentialState `json:"credential_state"`
	QueueState          QueueState      `json:"queue_state"`
	ActiveBatchID       string          `json:"active_batch_id,omitempty"`
	ActiveBatchState    BatchState      `json:"active_batch_state,omitempty"`
	BacklogFiles        int             `json:"backlog_files"`
	BacklogBytes        int64           `json:"backlog_bytes"`
	OrphanMedia         int             `json:"orphan_media"`
	Issues              int             `json:"issues"`
	DiskUsagePercent    int             `json:"disk_usage_percent"`
	InodeUsagePercent   int             `json:"inode_usage_percent"`
}

// StoragePauseThreshold pauses archive creation and producer writes.
const StoragePauseThreshold = 90

const cleanedManifestRetention = 24 * time.Hour

var errRemoteConflict = errors.New("remote object exists with different content")

// Consumer drives batches through planned -> archived -> uploaded ->
// verified -> cleaned/retained. Every step is persisted before the next one
// starts, so a restart resumes exactly where it stopped.
type Consumer struct {
	cfg   Config
	store Store
	now   func() time.Time
	// queueStats is used when cfg.QueueGuard.StatsPath is empty.
	queueStats func() (ProducerStats, bool)
	logf       func(format string, args ...any)

	mu          sync.Mutex
	nextAttempt map[string]time.Time
	index       map[string]manifestIndexEntry
	lastScan    ScanResult
	lastResult  CycleResult
	lastErr     string
	queueState  QueueState
	lastCycleAt time.Time
}

type manifestIndexEntry struct {
	modTime time.Time
	keys    [][16]byte
}

func NewConsumer(cfg Config, store Store) *Consumer {
	return &Consumer{
		cfg:         cfg,
		store:       store,
		now:         time.Now,
		logf:        func(string, ...any) {},
		nextAttempt: make(map[string]time.Time),
		index:       make(map[string]manifestIndexEntry),
	}
}

// SetLogger installs a log function; messages never contain credentials.
func (c *Consumer) SetLogger(logf func(format string, args ...any)) {
	if logf != nil {
		c.logf = logf
	}
}

// SetQueueStats installs an in-process producer stats source.
func (c *Consumer) SetQueueStats(fn func() (ProducerStats, bool)) { c.queueStats = fn }

// EnsureDirectories creates work_dir and its sub directories.
func EnsureDirectories(cfg Config) error {
	for _, dir := range []string{cfg.WorkDir, manifestDir(cfg.WorkDir), filepath.Join(cfg.WorkDir, "archives")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func sourceIdentity(e SourceEntry) [16]byte {
	h := sha256.New()
	h.Write([]byte(e.RootID))
	h.Write([]byte{0})
	h.Write([]byte(e.RelativePath))
	var b [16]byte
	binary.BigEndian.PutUint64(b[:8], uint64(e.Size))
	binary.BigEndian.PutUint64(b[8:], uint64(e.ModTimeNanos))
	h.Write(b[:])
	var out [16]byte
	copy(out[:], h.Sum(nil))
	return out
}

func (c *Consumer) evaluateQueue(now time.Time) (QueueState, error) {
	if c.cfg.QueueGuard.StatsPath == "" && c.queueStats != nil {
		stats, ok := c.queueStats()
		if !ok {
			return QueueDisabled, nil
		}
		return evaluateStats(stats, c.cfg.QueueGuard, now)
	}
	return EvaluateQueueGuard(c.cfg.QueueGuard, now)
}

// loadManifests returns all readable manifests sorted by creation time and
// refreshes the uploaded-source index.
func (c *Consumer) loadManifests(now time.Time) ([]BatchManifest, map[[16]byte]struct{}) {
	dir := manifestDir(c.cfg.WorkDir)
	entries, _ := os.ReadDir(dir)
	var manifests []BatchManifest
	seenFiles := make(map[string]struct{})
	known := make(map[[16]byte]struct{})
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasPrefix(name, ".") {
			continue
		}
		file := filepath.Join(dir, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}
		m, err := LoadManifest(file)
		if err != nil {
			c.logf("session recorder: quarantine manifest %s: %v", name, err)
			_ = QuarantineManifest(file)
			continue
		}
		if m.State == StateCleaned {
			if updated, err := time.Parse(time.RFC3339Nano, m.UpdatedAt); err == nil && now.Sub(updated) > cleanedManifestRetention {
				_ = os.Remove(file)
				continue
			}
		}
		seenFiles[file] = struct{}{}
		cached, ok := c.index[file]
		if !ok || !cached.modTime.Equal(info.ModTime()) {
			cached = manifestIndexEntry{modTime: info.ModTime()}
			for _, s := range m.Sources {
				cached.keys = append(cached.keys, sourceIdentity(s))
			}
			c.index[file] = cached
		}
		for _, k := range cached.keys {
			known[k] = struct{}{}
		}
		manifests = append(manifests, m)
	}
	for file := range c.index {
		if _, ok := seenFiles[file]; !ok {
			delete(c.index, file)
		}
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].CreatedAt < manifests[j].CreatedAt })
	return manifests, known
}

// RunCycle performs one bounded unit of work.
func (c *Consumer) RunCycle(ctx context.Context) CycleResult {
	now := c.now()
	result := CycleResult{State: "idle"}
	defer func() {
		c.mu.Lock()
		c.lastResult = result
		c.lastCycleAt = now
		c.mu.Unlock()
	}()
	if err := EnsureDirectories(c.cfg); err != nil {
		result.State = "filesystem_error"
		c.setErr(err)
		return result
	}
	queue, qerr := c.evaluateQueue(now)
	c.mu.Lock()
	c.queueState = queue
	c.mu.Unlock()
	if queue == QueueBusy || queue == QueueUnknown {
		if c.store != nil {
			c.store.SetBusy(true)
		}
		result.State = "queue_" + string(queue)
		if qerr != nil {
			c.setErr(qerr)
		}
		return result
	}
	manifests, known := c.loadManifests(now)
	for i := range manifests {
		m := &manifests[i]
		if m.State.Terminal() {
			continue
		}
		result.BatchID = m.BatchID
		if next, ok := c.nextAttempt[m.BatchID]; ok && now.Before(next) {
			result.State = "backoff"
			return result
		}
		c.advanceBatch(ctx, m, &result)
		return result
	}
	if !c.cfg.UploadEnabled {
		result.State = "upload_disabled"
		return result
	}
	if storageFull(c.cfg.WorkDir) {
		result.State = "storage_paused"
		return result
	}
	skip := func(e SourceEntry) bool {
		_, ok := known[sourceIdentity(e)]
		return ok
	}
	scan := ScanSpools(c.cfg, now, skip, c.cfg.QueueGuard.StatsPath)
	c.mu.Lock()
	c.lastScan = scan
	c.mu.Unlock()
	result.ScannedFiles = scan.ScannedFiles
	result.OrphanMedia = scan.OrphanMedia
	result.Issues = len(scan.Issues)
	if len(scan.Units) == 0 {
		return result
	}
	units, err := SelectUnitsByCompressedTarget(scan.Units, c.cfg)
	if err != nil || len(units) == 0 {
		result.State = "select_error"
		if err != nil {
			c.setErr(err)
		}
		return result
	}
	m, err := NewPlannedManifest(UnitSources(units), now, c.cfg)
	if err != nil {
		result.State = "plan_error"
		c.setErr(err)
		return result
	}
	if err := SaveManifest(c.cfg.WorkDir, m); err != nil {
		result.State = "filesystem_error"
		c.setErr(err)
		return result
	}
	result.BatchID = m.BatchID
	c.advanceBatch(ctx, &m, &result)
	return result
}

func (c *Consumer) setErr(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		c.lastErr = ""
		return
	}
	c.lastErr = err.Error()
}

func (c *Consumer) backoff(retry int) time.Duration {
	d := time.Duration(c.cfg.RetryInitialBackoffSeconds) * time.Second
	limit := time.Duration(c.cfg.RetryMaxBackoffSeconds) * time.Second
	for range min(retry, 20) {
		d *= 2
		if d >= limit {
			return limit
		}
	}
	return min(d, limit)
}

func (c *Consumer) fail(m *BatchManifest, category string, err error, result *CycleResult) {
	m.RetryCount++
	m.LastErrorCategory = category
	m.UpdatedAt = c.now().UTC().Format(timeLayout)
	_ = SaveManifest(c.cfg.WorkDir, *m)
	c.nextAttempt[m.BatchID] = c.now().Add(c.backoff(m.RetryCount - 1))
	result.State = category
	c.setErr(fmt.Errorf("batch %s: %s: %w", m.BatchID, category, err))
	c.logf("session recorder: batch %s %s: %v", m.BatchID, category, err)
}

// advanceBatch moves one batch as far as possible, persisting every step.
func (c *Consumer) advanceBatch(ctx context.Context, m *BatchManifest, result *CycleResult) {
	for !m.State.Terminal() {
		if ctx.Err() != nil {
			result.State = "stopping"
			return
		}
		switch m.State {
		case StatePlanned:
			if storageFull(c.cfg.WorkDir) {
				result.State = "storage_paused"
				return
			}
			if err := CreateArchive(m, c.cfg, c.now()); err != nil {
				if strings.HasPrefix(err.Error(), "source changed while archiving") || strings.HasPrefix(err.Error(), "open source") ||
					strings.HasPrefix(err.Error(), "source path contains symlink") {
					// The planned source set is no longer valid; drop the
					// plan and re-scan next cycle. Nothing was uploaded yet.
					_ = os.Remove(manifestPath(c.cfg.WorkDir, m.BatchID))
					result.State = "source_changed"
					c.setErr(err)
					return
				}
				c.fail(m, "archive_failed", err, result)
				return
			}
		case StateArchived:
			if c.store == nil {
				result.State = "upload_disabled"
				return
			}
			if err := c.EnsureUploaded(ctx, m); err != nil {
				category := "upload_failed"
				if errors.Is(err, errRemoteConflict) {
					category = "remote_conflict"
				}
				c.fail(m, category, err, result)
				return
			}
		case StateUploaded:
			if c.store == nil {
				result.State = "upload_disabled"
				return
			}
			if err := c.VerifyUploaded(ctx, m); err != nil {
				c.fail(m, "verify_failed", err, result)
				return
			}
			if m.ArchivePath != "" {
				_ = os.Remove(m.ArchivePath)
			}
		case StateVerified:
			if c.cfg.LocalCleanupEnabled {
				c.CleanupVerified(m)
			} else {
				for i := range m.Sources {
					if m.Sources[i].Cleanup == CleanupPending {
						m.Sources[i].Cleanup = CleanupRetained
					}
				}
				m.Transition(StateRetained, c.now())
			}
		default:
			c.fail(m, "invalid_state", fmt.Errorf("unknown state %q", m.State), result)
			return
		}
		m.LastErrorCategory = ""
		if err := SaveManifest(c.cfg.WorkDir, *m); err != nil {
			result.State = "filesystem_error"
			c.setErr(err)
			return
		}
	}
	delete(c.nextAttempt, m.BatchID)
	result.State = string(m.State)
	c.setErr(nil)
}

func remoteMatches(meta ObjectMetadata, m *BatchManifest) bool {
	return meta.Exists && meta.Size == m.ArchiveSize && strings.EqualFold(meta.Metadata[metaSHA256], m.ArchiveSHA256)
}

// EnsureUploaded uploads the archive unless an identical object exists.
func (c *Consumer) EnsureUploaded(ctx context.Context, m *BatchManifest) error {
	meta, err := c.store.Head(ctx, m.ObjectKey)
	if err != nil {
		return err
	}
	if meta.Exists {
		if !remoteMatches(meta, m) {
			return errRemoteConflict
		}
		m.Transition(StateUploaded, c.now())
		return nil
	}
	if _, err := os.Stat(m.ArchivePath); err != nil {
		// The local archive vanished (e.g. crash before rename); rebuild it.
		m.State = StatePlanned
		m.ArchivePath, m.ArchiveSize, m.ArchiveSHA256 = "", 0, ""
		return nil
	}
	err = c.store.PutFile(ctx, m.ObjectKey, m.ArchivePath, m.ArchiveSize, m.ArchiveSHA256)
	if errors.Is(err, ErrPreconditionFailed) {
		meta, herr := c.store.Head(ctx, m.ObjectKey)
		if herr != nil {
			return herr
		}
		if !remoteMatches(meta, m) {
			return errRemoteConflict
		}
		err = nil
	}
	if err != nil {
		return err
	}
	m.Transition(StateUploaded, c.now())
	return nil
}

// VerifyUploaded confirms size and sha256 metadata with HEAD.
func (c *Consumer) VerifyUploaded(ctx context.Context, m *BatchManifest) error {
	meta, err := c.store.Head(ctx, m.ObjectKey)
	if err != nil {
		return err
	}
	if !meta.Exists {
		m.State = StateArchived
		return errors.New("uploaded object not found")
	}
	if !remoteMatches(meta, m) {
		return errRemoteConflict
	}
	m.Transition(StateVerified, c.now())
	return nil
}

func cleanupPrivateName(batchID string, e SourceEntry) string {
	sum := sha256.Sum256([]byte(batchID + "\x00" + e.RootID + "\x00" + e.RelativePath))
	return ".session-recorder-cleanup-" + hex.EncodeToString(sum[:])
}

// CleanupVerified deletes the verified local sources. A file is first
// renamed into <root>/.session-recorder-cleanup under a deterministic
// private name, re-checked, and only then removed, so a file that changed
// after upload is never deleted. Crashes are recovered through the
// deterministic name.
func (c *Consumer) CleanupVerified(m *BatchManifest) {
	roots := rootPaths(c.cfg)
	dirs := make(map[string]time.Time)
	cutoff := c.now().Add(-time.Duration(c.cfg.StabilityWindowSeconds) * time.Second)
	for i := range m.Sources {
		e := &m.Sources[i]
		if e.Cleanup != CleanupPending {
			continue
		}
		rootPath, ok := roots[e.RootID]
		if !ok {
			e.Cleanup, e.ErrorCategory = CleanupFailed, "unknown_root"
			continue
		}
		parent := filepath.Dir(e.AbsolutePath)
		if _, seen := dirs[parent]; !seen {
			if info, err := os.Stat(parent); err == nil {
				dirs[parent] = info.ModTime()
			}
		}
		quarantine := filepath.Join(rootPath, cleanupDirName)
		private := filepath.Join(quarantine, cleanupPrivateName(m.BatchID, *e))
		e.CleanupPrivateName = filepath.Base(private)
		c.cleanupOne(e, rootPath, quarantine, private)
	}
	m.DeletedCount, m.SkippedCount, m.MissingCount, m.FailedCount = 0, 0, 0, 0
	for _, e := range m.Sources {
		switch e.Cleanup {
		case CleanupDeleted:
			m.DeletedCount++
		case CleanupSkippedChanged:
			m.SkippedCount++
		case CleanupMissing:
			m.MissingCount++
		case CleanupFailed:
			m.FailedCount++
		}
	}
	removeEmptyDirs(dirs, roots, cutoff)
	m.Transition(StateCleaned, c.now())
}

func sameFile(info fs.FileInfo, e *SourceEntry) bool {
	return info.Mode().IsRegular() && info.Size() == e.Size && info.ModTime().UnixNano() == e.ModTimeNanos
}

func (c *Consumer) cleanupOne(e *SourceEntry, rootPath, quarantine, private string) {
	info, err := os.Lstat(e.AbsolutePath)
	if errors.Is(err, fs.ErrNotExist) {
		// Recover an interrupted cleanup.
		if pinfo, perr := os.Lstat(private); perr == nil {
			if sameFile(pinfo, e) {
				if err := os.Remove(private); err != nil {
					e.Cleanup, e.ErrorCategory = CleanupFailed, "remove_failed"
					return
				}
				e.CleanupRenamed = true
				e.Cleanup = CleanupDeleted
				return
			}
			e.Cleanup, e.ErrorCategory = CleanupSkippedChanged, "unsafe_cleanup_quarantine"
			return
		}
		e.Cleanup = CleanupMissing
		return
	}
	if err != nil {
		e.Cleanup, e.ErrorCategory = CleanupFailed, "stat_error"
		return
	}
	if symlink, err := hasSymlinkComponent(rootPath, e.AbsolutePath); err != nil || symlink {
		e.Cleanup, e.ErrorCategory = CleanupFailed, "unsupported_file_type"
		return
	}
	if !sameFile(info, e) {
		e.Cleanup = CleanupSkippedChanged
		return
	}
	if err := os.MkdirAll(quarantine, 0o700); err != nil {
		e.Cleanup, e.ErrorCategory = CleanupFailed, "filesystem_error"
		return
	}
	if err := os.Rename(e.AbsolutePath, private); err != nil {
		e.Cleanup, e.ErrorCategory = CleanupFailed, "rename_failed"
		return
	}
	e.CleanupRenamed = true
	pinfo, err := os.Lstat(private)
	if err != nil || !sameFile(pinfo, e) {
		// Changed between the check and the rename: put it back untouched.
		if err := os.Rename(private, e.AbsolutePath); err != nil {
			e.Cleanup, e.ErrorCategory = CleanupFailed, "restore_failed"
			return
		}
		e.CleanupRenamed = false
		e.Cleanup = CleanupSkippedChanged
		return
	}
	e.CleanupDevice, e.CleanupInode = fileIdentity(pinfo)
	if err := os.Remove(private); err != nil {
		e.Cleanup, e.ErrorCategory = CleanupFailed, "remove_failed"
		return
	}
	e.Cleanup = CleanupDeleted
}

// removeEmptyDirs removes now-empty source directories that were already
// older than the stability window before cleanup, walking up to (but never
// removing) the spool root.
func removeEmptyDirs(dirs map[string]time.Time, roots map[string]string, cutoff time.Time) {
	paths := make([]string, 0, len(dirs))
	for dir, mod := range dirs {
		if mod.Before(cutoff) {
			paths = append(paths, dir)
		}
	}
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) > len(paths[j]) })
	for _, dir := range paths {
		for {
			var root string
			for _, r := range roots {
				if pathInside(r, dir) {
					root = r
					break
				}
			}
			if root == "" || filepath.Clean(dir) == filepath.Clean(root) {
				break
			}
			if os.Remove(dir) != nil { // fails when not empty
				break
			}
			dir = filepath.Dir(dir)
		}
	}
}

// Check performs the connectivity probe: HEAD, then a zero-byte conditional
// PUT of <prefix>/_checks/<installation_id>. An existing probe written by a
// different installation is reported and never overwritten.
func Check(ctx context.Context, cfg Config, store Store) error {
	key := CheckObjectKey(cfg.R2.ObjectPrefix, cfg.InstallationID)
	meta, err := store.Head(ctx, key)
	if err != nil {
		return err
	}
	if meta.Exists {
		if meta.Metadata[metaInstallation] != cfg.InstallationID {
			return errors.New("check object exists with a different installation")
		}
		return nil
	}
	err = store.PutBytes(ctx, key, nil, map[string]string{metaInstallation: cfg.InstallationID})
	if errors.Is(err, ErrPreconditionFailed) {
		meta, err = store.Head(ctx, key)
		if err == nil && meta.Metadata[metaInstallation] != cfg.InstallationID {
			return errors.New("check object exists with a different installation")
		}
	}
	return err
}

// Status builds the status report from the last cycle.
func (c *Consumer) Status(creds CredentialState) (StatusReport, CycleResult, string, time.Time) {
	c.mu.Lock()
	scan, result, lastErr, queue, at := c.lastScan, c.lastResult, c.lastErr, c.queueState, c.lastCycleAt
	c.mu.Unlock()
	report := StatusReport{
		SetupCompleted:      c.cfg.SetupCompleted,
		UploadEnabled:       c.cfg.UploadEnabled,
		LocalCleanupEnabled: c.cfg.LocalCleanupEnabled,
		CredentialState:     creds,
		QueueState:          queue,
		BacklogFiles:        scan.BacklogFiles,
		BacklogBytes:        scan.BacklogBytes,
		OrphanMedia:         scan.OrphanMedia,
		Issues:              len(scan.Issues),
	}
	if report.QueueState == "" {
		report.QueueState = QueueUnknown
	}
	if result.BatchID != "" {
		if m, err := LoadManifest(manifestPath(c.cfg.WorkDir, result.BatchID)); err == nil && !m.State.Terminal() {
			report.ActiveBatchID = m.BatchID
			report.ActiveBatchState = m.State
		}
	}
	report.DiskUsagePercent, report.InodeUsagePercent, _ = StorageUsage(c.cfg.WorkDir)
	return report, result, lastErr, at
}

// RecentIssues returns up to limit issues from the last scan.
func (c *Consumer) RecentIssues(limit int) []ScanIssue {
	c.mu.Lock()
	defer c.mu.Unlock()
	issues := c.lastScan.Issues
	if len(issues) > limit {
		issues = issues[:limit]
	}
	return append([]ScanIssue(nil), issues...)
}
