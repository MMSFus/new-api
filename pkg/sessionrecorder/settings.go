package sessionrecorder

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// OptionKey is the single options-table key holding Settings as JSON.
const OptionKey = "SessionRecorderSetting"

// Settings is the admin-editable configuration. It never contains
// credentials: those come from AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY or
// from files in CredentialsDir / $CREDENTIALS_DIRECTORY.
type Settings struct {
	// Enabled is the master switch. Nothing runs while it is false.
	Enabled bool `json:"enabled"`
	// ProducerEnabled records matching relay traffic into the spool.
	ProducerEnabled bool `json:"producer_enabled"`
	// ConsumerEnabled runs the embedded uploader. Leave it off to keep the
	// spool-only mode for the external session-recorder binary.
	ConsumerEnabled bool `json:"consumer_enabled"`

	// Producer.
	SpoolRoot          string       `json:"spool_root"`
	RootID             string       `json:"root_id"`
	QueueSize          int          `json:"queue_size"`
	MaxBodyBytes       int64        `json:"max_body_bytes"`
	RecordRequestBody  bool         `json:"record_request_body"`
	RecordResponseBody bool         `json:"record_response_body"`
	StatsPath          string       `json:"stats_path"`
	Filter             FilterConfig `json:"filter"`

	// Consumer, mirrors the standalone configuration.
	UploadEnabled              bool   `json:"upload_enabled"`
	LocalCleanupEnabled        bool   `json:"local_cleanup_enabled"`
	InstallationID             string `json:"installation_id"`
	SiteID                     string `json:"site_id"`
	HostID                     string `json:"host_id"`
	WorkDir                    string `json:"work_dir"`
	R2Endpoint                 string `json:"r2_endpoint"`
	R2Region                   string `json:"r2_region"`
	R2Bucket                   string `json:"r2_bucket"`
	R2ObjectPrefix             string `json:"r2_object_prefix"`
	CredentialsDir             string `json:"credentials_dir"`
	ScanIntervalSeconds        int    `json:"scan_interval_seconds"`
	StabilityWindowSeconds     int    `json:"stability_window_seconds"`
	MaxBatchFiles              int    `json:"max_batch_files"`
	MaxBatchInputBytes         int64  `json:"max_batch_input_bytes"`
	TargetArchiveBytes         int64  `json:"target_archive_bytes"`
	GzipLevel                  int    `json:"gzip_level"`
	MultipartConcurrency       int    `json:"multipart_concurrency"`
	MaxUploadMbps              int    `json:"max_upload_mbps"`
	AdaptiveUpload             bool   `json:"adaptive_upload"`
	MaxRetries                 int    `json:"max_retries"`
	RetryInitialBackoffSeconds int    `json:"retry_initial_backoff_seconds"`
	RetryMaxBackoffSeconds     int    `json:"retry_max_backoff_seconds"`
	BusyThresholdPercent       int    `json:"busy_threshold_percent"`
	FreshnessSeconds           int    `json:"freshness_seconds"`
	// ExternalStatsPath lets the consumer follow an external producer's
	// stats file; empty uses the embedded producer's in-memory stats.
	ExternalStatsPath string `json:"external_stats_path"`
}

// DefaultSettings: everything off, conservative limits.
func DefaultSettings() Settings {
	d := DefaultConfig()
	return Settings{
		SpoolRoot:                  "/data/session-recorder/spool",
		RootID:                     "primary",
		QueueSize:                  1024,
		MaxBodyBytes:               8 << 20,
		RecordRequestBody:          true,
		RecordResponseBody:         true,
		UploadEnabled:              true,
		LocalCleanupEnabled:        false,
		SiteID:                     d.SiteID,
		HostID:                     d.HostID,
		WorkDir:                    "/data/session-recorder/work",
		R2Region:                   d.R2.Region,
		R2ObjectPrefix:             d.R2.ObjectPrefix,
		ScanIntervalSeconds:        60,
		StabilityWindowSeconds:     d.StabilityWindowSeconds,
		MaxBatchFiles:              d.MaxBatchFiles,
		MaxBatchInputBytes:         d.MaxBatchInputBytes,
		TargetArchiveBytes:         d.TargetArchiveBytes,
		GzipLevel:                  d.GzipLevel,
		MultipartConcurrency:       2,
		MaxUploadMbps:              50,
		AdaptiveUpload:             d.AdaptiveUpload,
		MaxRetries:                 d.MaxRetries,
		RetryInitialBackoffSeconds: d.RetryInitialBackoffSeconds,
		RetryMaxBackoffSeconds:     d.RetryMaxBackoffSeconds,
		BusyThresholdPercent:       d.QueueGuard.BusyThreshold,
		FreshnessSeconds:           d.QueueGuard.FreshnessSeconds,
	}
}

// ParseSettings decodes the option value on top of the defaults.
func ParseSettings(raw string) (Settings, error) {
	s := DefaultSettings()
	if strings.TrimSpace(raw) == "" {
		return s, nil
	}
	if err := common.UnmarshalJsonStr(raw, &s); err != nil {
		return DefaultSettings(), err
	}
	return s, nil
}

// EffectiveStatsPath is where the embedded producer writes its stats file.
func (s Settings) EffectiveStatsPath() string {
	if strings.TrimSpace(s.StatsPath) != "" {
		return s.StatsPath
	}
	if s.SpoolRoot == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(filepath.Clean(s.SpoolRoot)), "session-recorder-stats.json")
}

// ToConfig converts settings to the standalone consumer configuration.
func (s Settings) ToConfig() Config {
	cfg := DefaultConfig()
	cfg.SetupCompleted = true
	cfg.UploadEnabled = s.UploadEnabled
	cfg.LocalCleanupEnabled = s.LocalCleanupEnabled
	cfg.InstallationID = s.InstallationID
	cfg.SiteID = strings.TrimSpace(s.SiteID)
	cfg.HostID = strings.TrimSpace(s.HostID)
	cfg.SpoolRoots = []SpoolRoot{{RootID: strings.TrimSpace(s.RootID), Path: filepath.Clean(s.SpoolRoot)}}
	cfg.WorkDir = filepath.Clean(s.WorkDir)
	cfg.R2 = R2Config{
		Endpoint:     strings.TrimSpace(s.R2Endpoint),
		Region:       strings.TrimSpace(s.R2Region),
		Bucket:       strings.TrimSpace(s.R2Bucket),
		ObjectPrefix: strings.Trim(strings.TrimSpace(s.R2ObjectPrefix), "/"),
	}
	cfg.StabilityWindowSeconds = s.StabilityWindowSeconds
	cfg.MaxBatchFiles = s.MaxBatchFiles
	cfg.MaxBatchInputBytes = s.MaxBatchInputBytes
	cfg.TargetArchiveBytes = s.TargetArchiveBytes
	cfg.GzipLevel = s.GzipLevel
	cfg.MultipartConcurrency = s.MultipartConcurrency
	cfg.MaxUploadMbps = s.MaxUploadMbps
	cfg.AdaptiveUpload = s.AdaptiveUpload
	cfg.MaxRetries = s.MaxRetries
	cfg.RetryInitialBackoffSeconds = s.RetryInitialBackoffSeconds
	cfg.RetryMaxBackoffSeconds = s.RetryMaxBackoffSeconds
	cfg.QueueGuard = QueueConfig{
		StatsPath:        strings.TrimSpace(s.ExternalStatsPath),
		BusyThreshold:    s.BusyThresholdPercent,
		FreshnessSeconds: s.FreshnessSeconds,
	}
	return cfg
}

// Validate checks the settings that matter for the enabled components.
func (s Settings) Validate() error {
	if _, err := CompileFilter(s.Filter); err != nil {
		return err
	}
	if !s.Enabled {
		return nil
	}
	if s.ProducerEnabled || s.ConsumerEnabled {
		if !filepath.IsAbs(s.SpoolRoot) {
			return errors.New("spool_root: must be absolute")
		}
		if !safeKeySegment(s.RootID) {
			return errors.New("root_id: must be a single safe path segment")
		}
	}
	if s.ProducerEnabled {
		if s.QueueSize < 1 || s.QueueSize > 1<<20 {
			return errors.New("queue_size: must be between 1 and 1048576")
		}
		if s.MaxBodyBytes < 0 {
			return errors.New("max_body_bytes: must not be negative")
		}
	}
	if s.ConsumerEnabled {
		if s.ScanIntervalSeconds < 5 {
			return errors.New("scan_interval_seconds: must be at least 5")
		}
		cfg := s.ToConfig()
		if cfg.InstallationID == "" {
			cfg.InstallationID = "pending"
		}
		return cfg.Validate()
	}
	return nil
}
