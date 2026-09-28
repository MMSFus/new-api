// Package sessionrecorder embeds the session-recorder producer and consumer
// into new-api.
//
// The producer writes relay sessions into a local spool that follows the
// "Session Recorder 文件 Spool 协议 v1". The consumer scans that spool,
// builds tar.gz batches and uploads them to Cloudflare R2. Archive layout,
// manifest JSON and object keys are byte-compatible with the standalone
// session-recorder binary, so the embedded consumer and the external binary
// can be swapped without changing anything stored in R2.
package sessionrecorder

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const configVersion = 1

// Config mirrors the standalone binary's configuration. Field order and JSON
// names are identical to the original so status and manifests stay aligned.
type Config struct {
	Version                    int         `json:"version"`
	SetupCompleted             bool        `json:"setup_completed"`
	UploadEnabled              bool        `json:"upload_enabled"`
	LocalCleanupEnabled        bool        `json:"local_cleanup_enabled"`
	InstallationID             string      `json:"installation_id"`
	SiteID                     string      `json:"site_id"`
	HostID                     string      `json:"host_id"`
	SpoolRoots                 []SpoolRoot `json:"spool_roots"`
	WorkDir                    string      `json:"work_dir"`
	R2                         R2Config    `json:"r2"`
	StabilityWindowSeconds     int         `json:"stability_window_seconds"`
	MaxBatchFiles              int         `json:"max_batch_files"`
	MaxBatchInputBytes         int64       `json:"max_batch_input_bytes"`
	TargetArchiveBytes         int64       `json:"target_archive_bytes"`
	GzipLevel                  int         `json:"gzip_level"`
	ArchiveConcurrency         int         `json:"archive_concurrency"`
	MultipartConcurrency       int         `json:"multipart_concurrency"`
	MaxUploadMbps              int         `json:"max_upload_mbps"`
	AdaptiveUpload             bool        `json:"adaptive_upload"`
	MaxRetries                 int         `json:"max_retries"`
	RetryInitialBackoffSeconds int         `json:"retry_initial_backoff_seconds"`
	RetryMaxBackoffSeconds     int         `json:"retry_max_backoff_seconds"`
	ShutdownTimeoutSeconds     int         `json:"shutdown_timeout_seconds"`
	QueueGuard                 QueueConfig `json:"queue_guard"`
}

type R2Config struct {
	Endpoint     string `json:"endpoint"`
	Region       string `json:"region"`
	Bucket       string `json:"bucket"`
	ObjectPrefix string `json:"object_prefix"`
}

type QueueConfig struct {
	StatsPath        string `json:"stats_path,omitempty"`
	BusyThreshold    int    `json:"busy_threshold_percent"`
	FreshnessSeconds int    `json:"freshness_seconds"`
}

type SpoolRoot struct {
	RootID string `json:"root_id"`
	Path   string `json:"path"`
}

// DefaultConfig returns the same defaults as the standalone binary.
func DefaultConfig() Config {
	host, _ := os.Hostname()
	return Config{
		Version:             configVersion,
		UploadEnabled:       true,
		LocalCleanupEnabled: true,
		SiteID:              "default",
		HostID:              host,
		SpoolRoots:          []SpoolRoot{{RootID: "default", Path: "/var/lib/session-recorder/spool"}},
		WorkDir:             "/var/lib/session-recorder/work",
		R2: R2Config{
			Region:       "auto",
			ObjectPrefix: "standalone-v1",
		},
		StabilityWindowSeconds:     900,
		MaxBatchFiles:              5000,
		MaxBatchInputBytes:         2 << 30,
		TargetArchiveBytes:         1 << 30,
		GzipLevel:                  1,
		ArchiveConcurrency:         1,
		MultipartConcurrency:       4,
		AdaptiveUpload:             true,
		MaxRetries:                 5,
		RetryInitialBackoffSeconds: 5,
		RetryMaxBackoffSeconds:     300,
		ShutdownTimeoutSeconds:     120,
		QueueGuard: QueueConfig{
			BusyThreshold:    25,
			FreshnessSeconds: 120,
		},
	}
}

// safeKeySegment reports whether s can be used as exactly one object key or
// archive path segment.
func safeKeySegment(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" || t == "." || t == ".." {
		return false
	}
	return !strings.ContainsAny(t, `/\"`)
}

// NewInstallationID returns 16 random bytes as lowercase hex.
func NewInstallationID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// Validate applies the standalone binary's validation rules, in the same
// order and with the same messages.
func (c Config) Validate() error {
	if c.Version != configVersion {
		return fmt.Errorf("version: expected %d", configVersion)
	}
	if c.LocalCleanupEnabled && !c.UploadEnabled {
		return errors.New("local_cleanup_enabled: cannot be enabled when upload_enabled is false")
	}
	if strings.TrimSpace(c.InstallationID) == "" {
		return errors.New("installation_id: required")
	}
	if strings.TrimSpace(c.SiteID) == "" || strings.TrimSpace(c.HostID) == "" {
		return errors.New("site_id and host_id: required")
	}
	if !safeKeySegment(c.SiteID) || !safeKeySegment(c.HostID) {
		return errors.New("site_id and host_id: must be single safe path segments")
	}
	if !filepath.IsAbs(c.WorkDir) {
		return errors.New("work_dir: must be absolute")
	}
	if len(c.SpoolRoots) == 0 {
		return errors.New("spool_roots: at least one root is required")
	}
	seen := make(map[string]struct{}, len(c.SpoolRoots))
	for i, root := range c.SpoolRoots {
		if strings.TrimSpace(root.RootID) == "" {
			return fmt.Errorf("spool_roots[%d].root_id: required", i)
		}
		if !safeKeySegment(root.RootID) {
			return fmt.Errorf("spool_roots[%d].root_id: must be a single safe path segment", i)
		}
		if _, dup := seen[root.RootID]; dup {
			return fmt.Errorf("spool_roots[%d].root_id: duplicate", i)
		}
		seen[root.RootID] = struct{}{}
		if !filepath.IsAbs(root.Path) {
			return fmt.Errorf("spool_roots[%d].path: must be absolute", i)
		}
		if pathsOverlap(root.Path, c.WorkDir) {
			return fmt.Errorf("spool_roots[%d].path: must not overlap work_dir", i)
		}
	}
	prefix := strings.TrimSpace(c.R2.ObjectPrefix)
	if prefix == "" || strings.HasPrefix(prefix, "/") {
		return errors.New("r2.object_prefix: must be a non-empty relative prefix")
	}
	if c.UploadEnabled && (strings.TrimSpace(c.R2.Endpoint) == "" || strings.TrimSpace(c.R2.Bucket) == "") {
		return errors.New("r2.endpoint and r2.bucket: required when upload is enabled")
	}
	for segment := range strings.SplitSeq(prefix, "/") {
		if !safeKeySegment(segment) {
			return errors.New("r2.object_prefix: contains an unsafe path segment")
		}
	}
	if c.MaxBatchFiles <= 0 || c.MaxBatchInputBytes <= 0 || c.TargetArchiveBytes <= 0 || c.StabilityWindowSeconds <= 0 {
		return errors.New("batch limits: must be positive")
	}
	if c.GzipLevel < 1 || c.GzipLevel > 9 {
		return errors.New("gzip_level: must be between 1 and 9")
	}
	if c.ArchiveConcurrency <= 0 || c.MultipartConcurrency <= 0 || c.MaxUploadMbps < 0 {
		return errors.New("concurrency and max_upload_mbps: invalid")
	}
	if c.MaxRetries < 0 || c.RetryInitialBackoffSeconds <= 0 || c.RetryMaxBackoffSeconds < c.RetryInitialBackoffSeconds || c.ShutdownTimeoutSeconds <= 0 {
		return errors.New("retry or shutdown settings: invalid")
	}
	if c.QueueGuard.BusyThreshold < 0 || c.QueueGuard.BusyThreshold > 100 || c.QueueGuard.FreshnessSeconds <= 0 {
		return errors.New("queue_guard: invalid")
	}
	return nil
}

func pathsOverlap(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	return pathInside(a, b) || pathInside(b, a)
}

// pathInside reports whether target equals base or lives below it.
func pathInside(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
