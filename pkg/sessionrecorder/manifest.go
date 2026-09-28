package sessionrecorder

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type BatchState string

const (
	StatePlanned  BatchState = "planned"
	StateArchived BatchState = "archived"
	StateUploaded BatchState = "uploaded"
	StateVerified BatchState = "verified"
	StateCleaned  BatchState = "cleaned"
	StateRetained BatchState = "retained"
)

// Terminal reports whether no further work is required for the batch.
func (s BatchState) Terminal() bool {
	return s == StateCleaned || s == StateRetained
}

type CleanupDisposition string

const (
	CleanupPending        CleanupDisposition = "pending"
	CleanupDeleted        CleanupDisposition = "deleted"
	CleanupSkippedChanged CleanupDisposition = "skipped_changed"
	CleanupFailed         CleanupDisposition = "failed"
	CleanupMissing        CleanupDisposition = "missing"
	CleanupRetained       CleanupDisposition = "retained"
)

const timeLayout = "2006-01-02T15:04:05.999999999Z07:00"

// SourceEntry is one spool file included in a batch.
type SourceEntry struct {
	RootID             string             `json:"root_id"`
	AbsolutePath       string             `json:"absolute_path"`
	RelativePath       string             `json:"relative_path"`
	Size               int64              `json:"size"`
	ModTimeNanos       int64              `json:"mod_time_unix_nano"`
	Mode               uint32             `json:"mode"`
	Cleanup            CleanupDisposition `json:"cleanup"`
	ErrorCategory      string             `json:"error_category,omitempty"`
	CleanupPrivateName string             `json:"cleanup_private_name,omitempty"`
	CleanupRenamed     bool               `json:"cleanup_renamed,omitempty"`
	CleanupDevice      uint64             `json:"cleanup_device,omitempty"`
	CleanupInode       uint64             `json:"cleanup_inode,omitempty"`
}

// BatchManifest is the persistent, restart-safe state of one batch. It is
// stored under work_dir/manifests/<batch_id>.json.
type BatchManifest struct {
	FormatVersion     int           `json:"format_version"`
	BatchID           string        `json:"batch_id"`
	State             BatchState    `json:"state"`
	SiteID            string        `json:"site_id"`
	HostID            string        `json:"host_id"`
	CreatedAt         string        `json:"created_at"`
	UpdatedAt         string        `json:"updated_at"`
	Sources           []SourceEntry `json:"sources"`
	ArchivePath       string        `json:"archive_path,omitempty"`
	ArchiveSize       int64         `json:"archive_size,omitempty"`
	ArchiveSHA256     string        `json:"archive_sha256,omitempty"`
	ObjectKey         string        `json:"object_key"`
	DeletedCount      int           `json:"deleted_count"`
	SkippedCount      int           `json:"skipped_count"`
	MissingCount      int           `json:"missing_count"`
	FailedCount       int           `json:"failed_count"`
	LastErrorCategory string        `json:"last_error_category,omitempty"`
	RetryCount        int           `json:"retry_count"`
}

// batchFingerprint is hashed to derive the batch id. Field names and order
// are part of the on-wire compatibility contract.
type batchFingerprint struct {
	Site    string        `json:"site"`
	Host    string        `json:"host"`
	Sources []SourceEntry `json:"sources"`
}

// NewPlannedManifest creates a planned batch. The batch id is the first 16
// bytes of sha256 over the JSON fingerprint, and the object key is
// <prefix>/<site>/<host>/<YYYY/MM/DD/HH>/<batch_id>.tar.gz.
func NewPlannedManifest(sources []SourceEntry, now time.Time, cfg Config) (BatchManifest, error) {
	if len(sources) == 0 {
		return BatchManifest{}, errors.New("sources: empty")
	}
	sorted := append([]SourceEntry(nil), sources...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].RootID != sorted[j].RootID {
			return sorted[i].RootID < sorted[j].RootID
		}
		return sorted[i].RelativePath < sorted[j].RelativePath
	})
	for i := range sorted {
		sorted[i].Cleanup = CleanupPending
	}
	raw, err := common.Marshal(batchFingerprint{Site: cfg.SiteID, Host: cfg.HostID, Sources: sorted})
	if err != nil {
		return BatchManifest{}, err
	}
	sum := sha256.Sum256(raw)
	batchID := hex.EncodeToString(sum[:16])
	now = now.UTC()
	stamp := now.Format(timeLayout)
	return BatchManifest{
		FormatVersion: 1,
		BatchID:       batchID,
		State:         StatePlanned,
		SiteID:        cfg.SiteID,
		HostID:        cfg.HostID,
		CreatedAt:     stamp,
		UpdatedAt:     stamp,
		Sources:       sorted,
		ObjectKey:     ObjectKey(cfg.R2.ObjectPrefix, cfg.SiteID, cfg.HostID, now, batchID),
	}, nil
}

// ObjectKey builds the archive object key exactly like the standalone binary.
func ObjectKey(prefix, site, host string, t time.Time, batchID string) string {
	return path.Clean(fmt.Sprintf("%s/%s/%s/%s/%s.tar.gz", prefix, site, host, t.UTC().Format("2006/01/02/15"), batchID))
}

// CheckObjectKey is the zero-byte probe written by the connectivity check.
func CheckObjectKey(prefix, installationID string) string {
	return path.Join(prefix, "_checks", installationID)
}

// Transition moves the batch into a new state and stamps updated_at.
func (m *BatchManifest) Transition(next BatchState, now time.Time) {
	m.State = next
	m.UpdatedAt = now.UTC().Format(timeLayout)
}

func manifestDir(workDir string) string {
	return filepath.Join(workDir, "manifests")
}

func manifestPath(workDir, batchID string) string {
	return filepath.Join(manifestDir(workDir), batchID+".json")
}

// SaveManifest atomically persists the manifest.
func SaveManifest(workDir string, m BatchManifest) error {
	raw, err := common.Marshal(m)
	if err != nil {
		return err
	}
	return atomicWrite(manifestPath(workDir, m.BatchID), raw, 0o600)
}

// LoadManifest reads a manifest file and checks its basic invariants.
func LoadManifest(file string) (BatchManifest, error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return BatchManifest{}, err
	}
	var m BatchManifest
	if err := common.Unmarshal(raw, &m); err != nil {
		return BatchManifest{}, fmt.Errorf("manifest malformed: %w", err)
	}
	if m.FormatVersion != 1 || m.BatchID == "" || m.State == "" || len(m.Sources) == 0 {
		return BatchManifest{}, errors.New("manifest malformed: missing required fields")
	}
	if filepath.Base(file) != m.BatchID+".json" {
		return BatchManifest{}, errors.New("manifest malformed: batch id mismatch")
	}
	return m, nil
}

// QuarantineManifest renames a corrupt manifest out of the scan set.
func QuarantineManifest(file string) error {
	return os.Rename(file, file+".corrupt")
}

func atomicWrite(target string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	_ = tmp.Chmod(perm) // best effort; not supported on every platform
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, target)
}
