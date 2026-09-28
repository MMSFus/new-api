package sessionrecorder

import (
	"errors"
	"os"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type QueueState string

const (
	QueueHealthy  QueueState = "healthy"
	QueueBusy     QueueState = "busy"
	QueueDisabled QueueState = "disabled"
	QueueUnknown  QueueState = "unknown"
)

// ProducerStats is the queue stats file written by the producer.
type ProducerStats struct {
	Timestamp               string `json:"timestamp"`
	ExportQueueUsagePercent int    `json:"export_queue_usage_percent"`
}

// EvaluateQueueGuard reads the producer stats file. Uploads pause while the
// producer queue is busy or its state is unknown.
func EvaluateQueueGuard(cfg QueueConfig, now time.Time) (QueueState, error) {
	if cfg.StatsPath == "" {
		return QueueDisabled, nil
	}
	raw, err := os.ReadFile(cfg.StatsPath)
	if err != nil {
		return QueueUnknown, errors.New("queue stats unavailable")
	}
	var stats ProducerStats
	if err := common.Unmarshal(raw, &stats); err != nil {
		return QueueUnknown, errors.New("queue stats malformed")
	}
	return evaluateStats(stats, cfg, now)
}

func evaluateStats(stats ProducerStats, cfg QueueConfig, now time.Time) (QueueState, error) {
	ts, err := time.Parse(time.RFC3339Nano, stats.Timestamp)
	if err != nil {
		return QueueUnknown, errors.New("queue stats timestamp malformed")
	}
	if stats.ExportQueueUsagePercent < 0 || stats.ExportQueueUsagePercent > 100 {
		return QueueUnknown, errors.New("queue stats usage out of range")
	}
	if now.Sub(ts) > time.Duration(cfg.FreshnessSeconds)*time.Second {
		return QueueUnknown, errors.New("queue stats stale")
	}
	if stats.ExportQueueUsagePercent >= cfg.BusyThreshold {
		return QueueBusy, nil
	}
	return QueueHealthy, nil
}

// StorageUsage returns disk and inode usage percentages for path. The inode
// percentage is 0 where the platform has no inode concept.
func StorageUsage(path string) (disk int, inode int, err error) {
	return storageUsageFn(path)
}

// storageUsageFn is replaceable in tests.
var storageUsageFn = storageUsage

// storageFull reports whether disk or inode usage of path reached
// StoragePauseThreshold. Unknown usage never pauses.
func storageFull(path string) bool {
	disk, inode, err := StorageUsage(path)
	return err == nil && (disk >= StoragePauseThreshold || inode >= StoragePauseThreshold)
}
