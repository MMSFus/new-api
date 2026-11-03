package model

import (
	"math"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/tidwall/sjson"
	"gorm.io/gorm"
)

const OnlySub2APIRatioSync = "Only Sub2API channels support upstream ratio sync"

type ChannelCostRatioHistory struct {
	ID           int      `json:"id"`
	ChannelID    int      `json:"channel_id" gorm:"index:idx_channel_cost_ratio_time,priority:1"`
	OldRatio     *float64 `json:"old_ratio"`
	NewRatio     *float64 `json:"new_ratio"`
	Currency     string   `json:"currency" gorm:"type:varchar(16)"`
	Source       string   `json:"source" gorm:"type:varchar(16)"`
	OperatorID   int      `json:"operator_id"`
	UpstreamInfo string   `json:"upstream_info" gorm:"type:text"`
	CreatedAt    int64    `json:"created_at" gorm:"index:idx_channel_cost_ratio_time,priority:2"`
}

type ChannelCostRatioSyncStatus struct {
	ChannelID     int      `json:"-" gorm:"primaryKey;autoIncrement:false"`
	Status        string   `json:"status" gorm:"type:varchar(32)"`
	LastAttemptAt int64    `json:"last_attempt_at"`
	LastSuccessAt int64    `json:"last_success_at"`
	LastValue     *float64 `json:"last_value"`
	Error         string   `json:"error" gorm:"type:text"`
}

type ChannelCostRatioSyncResult struct {
	Status   string   `json:"status"`
	OldRatio *float64 `json:"old_ratio"`
	NewRatio *float64 `json:"new_ratio"`
	Value    *float64 `json:"value"`
	Error    string   `json:"error"`
}

func ValidateUpstreamCostSettings(channelType int, s dto.ChannelSettings) error {
	switch s.UpstreamCostCurrency {
	case "", "USD", "CNY", "CUSTOM":
	default:
		return common.NewMessage("Invalid upstream cost currency")
	}
	if s.UpstreamCostRatioSyncEnabled && channelType != constant.ChannelTypeSub2API {
		return common.NewMessage(OnlySub2APIRatioSync)
	}
	for _, bound := range []*float64{s.UpstreamCostRatioSyncMin, s.UpstreamCostRatioSyncMax} {
		if bound != nil && !(*bound >= 0 && *bound <= dto.MaxUpstreamCostRatio) {
			return common.NewMessage("Upstream ratio sync bounds must be between 0 and 100")
		}
	}
	if s.UpstreamCostRatioSyncMin != nil && s.UpstreamCostRatioSyncMax != nil && *s.UpstreamCostRatioSyncMin > *s.UpstreamCostRatioSyncMax {
		return common.NewMessage("Upstream ratio sync minimum must not exceed maximum")
	}
	return nil
}

func UpstreamCostRatioInRange(s dto.ChannelSettings, value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= dto.MaxUpstreamCostRatio &&
		(s.UpstreamCostRatioSyncMin == nil || value >= *s.UpstreamCostRatioSyncMin) &&
		(s.UpstreamCostRatioSyncMax == nil || value <= *s.UpstreamCostRatioSyncMax)
}

func recordChannelCostRatioChange(tx *gorm.DB, old dto.ChannelSettings, channel Channel, source, upstreamInfo string) error {
	next := channel.GetSetting()
	if err := ValidateUpstreamCostSettings(channel.Type, next); err != nil {
		return err
	}
	sameRatio := old.UpstreamCostRatio == nil && next.UpstreamCostRatio == nil ||
		old.UpstreamCostRatio != nil && next.UpstreamCostRatio != nil && *old.UpstreamCostRatio == *next.UpstreamCostRatio
	if sameRatio && old.UpstreamCostCurrency == next.UpstreamCostCurrency {
		return nil
	}
	return tx.Create(&ChannelCostRatioHistory{
		ChannelID: channel.Id, OldRatio: old.UpstreamCostRatio, NewRatio: next.UpstreamCostRatio,
		Currency: next.UpstreamCostCurrency, Source: source, OperatorID: channel.CostRatioOperatorID,
		UpstreamInfo: upstreamInfo, CreatedAt: common.GetTimestamp(),
	}).Error
}

func GetChannelCostRatioHistory(channelID, page, pageSize int) ([]ChannelCostRatioHistory, int64, *ChannelCostRatioSyncStatus, error) {
	items := make([]ChannelCostRatioHistory, 0)
	var total int64
	query := DB.Model(&ChannelCostRatioHistory{}).Where("channel_id = ?", channelID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, nil, err
	}
	if err := query.Order("created_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, nil, err
	}
	var status ChannelCostRatioSyncStatus
	result := DB.Where("channel_id = ?", channelID).Limit(1).Find(&status)
	if result.Error != nil {
		return nil, 0, nil, result.Error
	}
	if result.RowsAffected == 0 {
		return items, total, nil, nil
	}
	return items, total, &status, nil
}

// ApplyChannelCostRatioSync serializes settings, history and attempt state with
// channel edits. It reads settings again after the network call, so a sync never
// replaces unrelated settings or bypasses a newly changed range.
func ApplyChannelCostRatioSync(channelID int, value *float64, status, message, upstreamInfo string, automatic bool) (result ChannelCostRatioSyncResult, err error) {
	var channel Channel
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&channel, channelID).Error; err != nil {
			return err
		}
		if channel.Type != constant.ChannelTypeSub2API {
			return common.NewMessage(OnlySub2APIRatioSync)
		}
		settings := channel.GetSetting()
		result = ChannelCostRatioSyncResult{Status: status, OldRatio: settings.UpstreamCostRatio, NewRatio: settings.UpstreamCostRatio, Value: value, Error: message}
		if automatic && !settings.UpstreamCostRatioSyncEnabled {
			return nil
		}
		if value != nil && status == "success" {
			if !UpstreamCostRatioInRange(settings, *value) {
				result.Status = "out_of_range"
			} else if settings.UpstreamCostRatio != nil && *settings.UpstreamCostRatio == *value {
				result.Status = "unchanged"
			} else {
				// Preserve even settings unknown to this version of the gateway.
				raw := "{}"
				if channel.Setting != nil && *channel.Setting != "" {
					raw = *channel.Setting
				}
				updated, err := sjson.Set(raw, "upstream_cost_ratio", *value)
				if err != nil {
					return err
				}
				channel.Setting = &updated
				if err := tx.Model(&Channel{}).Where("id = ?", channelID).UpdateColumn("setting", updated).Error; err != nil {
					return err
				}
				if err := recordChannelCostRatioChange(tx, settings, channel, "sync", upstreamInfo); err != nil {
					return err
				}
				result.NewRatio = value
			}
		}
		var state ChannelCostRatioSyncStatus
		if err := tx.Where("channel_id = ?", channelID).Limit(1).Find(&state).Error; err != nil {
			return err
		}
		state.ChannelID, state.Status, state.Error = channelID, result.Status, result.Error
		state.LastAttemptAt, state.LastValue = common.GetTimestamp(), value
		if result.Status == "success" || result.Status == "unchanged" {
			state.LastSuccessAt = state.LastAttemptAt
		}
		return tx.Save(&state).Error
	})
	if err == nil {
		CacheUpdateChannel(&channel)
	}
	return
}
