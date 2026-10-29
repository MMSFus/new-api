package model

import (
	"context"
	"errors"
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"gorm.io/gorm"
)

const ChannelCostBackfillBatchSize = 100

type ChannelCostBackfillCursor struct {
	CreatedAt int64 `json:"created_at"`
	ID        int   `json:"id"`
}

type ChannelCostBackfillState struct {
	Total     int64            `json:"total"`
	Processed int64            `json:"processed"`
	Updated   int64            `json:"updated"`
	Skipped   int64            `json:"skipped"`
	Progress  int              `json:"progress"`
	Reasons   map[string]int64 `json:"reasons"`
}

func ChannelCostBackfillSupported() bool {
	return !common.UsingLogDatabase(common.DatabaseTypeClickHouse)
}

// PrepareChannelCostBackfill captures the scan boundary and current channel
// ratios once. It runs in the worker, never in the management request.
func PrepareChannelCostBackfill(ctx context.Context, filter LogFilter) (upperID int, total int64, ratios map[int]*float64, err error) {
	if !ChannelCostBackfillSupported() {
		return 0, 0, nil, errors.New("Historical cost backfill is not supported by this log database")
	}
	var last Log
	err = LOG_DB.WithContext(ctx).Select("id").Order("id DESC").Limit(1).Find(&last).Error
	if err != nil {
		return
	}
	upperID = last.Id
	filter.Type, filter.Types = LogTypeUnknown, channelCostLogTypes
	var tx *gorm.DB
	tx, err = filter.apply(LOG_DB.WithContext(ctx).Model(&Log{}))
	if err != nil {
		return
	}
	err = tx.Where("logs.id <= ?", upperID).Count(&total).Error
	if err != nil {
		return
	}
	var channels []Channel
	err = DB.WithContext(ctx).Select("id", "setting").Find(&channels).Error
	if err != nil {
		return
	}
	ratios = make(map[int]*float64, len(channels))
	for _, channel := range channels {
		ratios[channel.Id] = channel.GetSetting().UpstreamCostRatio
	}
	return
}

// HistoricalChannelCost uses the same official-price basis as live snapshots.
// group_ratio already includes any user-group override in relay logs. Missing
// historical billing facts must never be substituted with today's group prices.
func HistoricalChannelCost(log Log, ratios map[int]*float64, taskID string, timestamp int64) (string, string) {
	if !gjson.Valid(log.Other) || !gjson.Parse(log.Other).IsObject() {
		return "", "Invalid historical billing data"
	}
	snapshot := gjson.Get(log.Other, "root_info.channel_cost")
	if snapshot.Get("cost_quota").Exists() {
		return "", ""
	}
	root := gjson.Get(log.Other, "root_info")
	if snapshot.Exists() || (root.Exists() && !root.IsObject()) {
		return "", "Invalid historical billing data"
	}
	ratio, exists := ratios[log.ChannelId]
	if !exists {
		return "", "Channel no longer exists"
	}
	if ratio == nil {
		return "", "Channel cost ratio is not configured"
	}
	group := gjson.Get(log.Other, "group_ratio")
	if group.Type != gjson.Number || !(group.Float() > 0) || math.IsInf(group.Float(), 0) ||
		math.IsNaN(*ratio) || math.IsInf(*ratio, 0) || *ratio < 0 || log.Quota < 0 {
		return "", "Invalid historical billing data"
	}
	basis := float64(log.Quota)
	if audit := gjson.Get(log.Other, "admin_info.simulated_cache"); audit.Exists() {
		original := audit.Get("original.quota")
		if original.Type != gjson.Number || original.Float() < 0 || original.Float() > float64(common.MaxQuota) || math.Trunc(original.Float()) != original.Float() {
			return "", "Invalid historical billing data"
		}
		basis = original.Float()
	}
	base := basis / group.Float()
	cost := base * *ratio
	if math.IsNaN(base) || math.IsInf(base, 0) || math.IsNaN(cost) || math.IsInf(cost, 0) {
		return "", "Invalid historical billing data"
	}
	other, err := sjson.Set(log.Other, "root_info.channel_cost", ChannelCostSnapshot{
		UpstreamRatio: *ratio, GroupRatio: group.Float(), BaseQuota: base, CostQuota: cost,
		Source: "historical_backfill", BackfilledAt: timestamp, TaskID: taskID,
	})
	if err != nil {
		return "", "Invalid historical billing data"
	}
	return other, ""
}

// BackfillChannelCostBatch holds locks only for a bounded batch. Keyset paging
// uses the existing (created_at, id) index; there is no OFFSET or JSON SQL scan.
// Reading under the lock preserves concurrent metadata updates and snapshots.
func BackfillChannelCostBatch(ctx context.Context, filter LogFilter, upperID int, cursor ChannelCostBackfillCursor, ratios map[int]*float64, taskID string) (next ChannelCostBackfillCursor, delta ChannelCostBackfillState, err error) {
	next = cursor
	delta.Reasons = map[string]int64{}
	if !ChannelCostBackfillSupported() {
		return next, delta, errors.New("Historical cost backfill is not supported by this log database")
	}
	filter.Type, filter.Types = LogTypeUnknown, channelCostLogTypes
	err = LOG_DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query, err := filter.apply(tx.Model(&Log{}))
		if err != nil {
			return err
		}
		query = query.Where("logs.id <= ?", upperID)
		if cursor.ID != 0 {
			query = query.Where("logs.created_at > ? OR (logs.created_at = ? AND logs.id > ?)", cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
		}
		var logs []Log
		if err := lockForUpdate(query).Select("logs.id, logs.created_at, logs.channel_id, logs.quota, logs.other").Order("logs.created_at ASC, logs.id ASC").Limit(ChannelCostBackfillBatchSize).Find(&logs).Error; err != nil {
			return err
		}
		for _, log := range logs {
			next = ChannelCostBackfillCursor{CreatedAt: log.CreatedAt, ID: log.Id}
			delta.Processed++
			other, reason := HistoricalChannelCost(log, ratios, taskID, common.GetTimestamp())
			if reason != "" {
				delta.Skipped++
				delta.Reasons[reason]++
				continue
			}
			if other == "" {
				continue
			}
			if err := tx.Model(&Log{}).Where("id = ?", log.Id).UpdateColumn("other", other).Error; err != nil {
				return err
			}
			delta.Updated++
		}
		return nil
	})
	return
}
