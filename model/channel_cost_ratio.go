package model

import (
	"math"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/tidwall/sjson"
	"gorm.io/gorm"
)

const (
	upstreamRatioNeedsCustomBaseURL = "Upstream ratio sync needs a custom base URL"
	upstreamRatioOfficialAPI        = "Official provider APIs do not expose upstream ratios"
	upstreamRatioCredentials        = "This channel's credentials cannot query upstream ratios"
)

var upstreamRatioOfficialDomains = []string{
	"anthropic.com", "openai.com", "googleapis.com", "x.ai", "grok.com",
	"ollama.com", "moonshot.cn", "kimi.com", "bigmodel.cn", "deepseek.com",
	"azure.com", "amazonaws.com", "cloudflare.com", "volces.com", "aliyuncs.com",
}

// UpstreamRatioSyncEligibility determines whether a channel can safely query
// the custom upstream billing endpoint with its ordinary API key.
func UpstreamRatioSyncEligibility(channel *Channel) (bool, string) {
	return upstreamRatioSyncEligibility(channel, true)
}

func upstreamRatioSyncEligibility(channel *Channel, requireKey bool) (bool, string) {
	if channel == nil {
		return false, upstreamRatioNeedsCustomBaseURL
	}
	baseURL := strings.TrimSpace(channel.GetBaseURL())
	parsed, err := url.Parse(baseURL)
	if baseURL == "" || err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return false, upstreamRatioNeedsCustomBaseURL
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	for _, domain := range upstreamRatioOfficialDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return false, upstreamRatioOfficialAPI
		}
	}
	if channel.Type == constant.ChannelTypeCodex || channel.Type == constant.ChannelTypeAntigravity ||
		channel.Type == constant.ChannelTypeKimi || channel.Type == constant.ChannelTypeVertexAi ||
		channel.Type == constant.ChannelTypeAws || strings.HasPrefix(strings.TrimSpace(channel.Key), "{") {
		return false, upstreamRatioCredentials
	}
	if requireKey && strings.TrimSpace(channel.Key) == "" {
		return false, upstreamRatioCredentials
	}
	return true, ""
}

type ChannelCostRatioHistory struct {
	ID           int      `json:"id"`
	ChannelID    int      `json:"channel_id" gorm:"index:idx_channel_cost_ratio_time,priority:1"`
	OldRatio     *float64 `json:"old_ratio"`
	NewRatio     *float64 `json:"new_ratio"`
	OldCurrency  string   `json:"old_currency" gorm:"type:varchar(16)"`
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

func ValidateUpstreamCostSettings(channel *Channel, s dto.ChannelSettings) error {
	switch s.UpstreamCostCurrency {
	case "", "USD", "CNY", "CUSTOM":
	default:
		return common.NewMessage("Invalid upstream cost currency")
	}
	if s.UpstreamCostRatioSyncEnabled {
		if eligible, reason := upstreamRatioSyncEligibility(channel, false); !eligible {
			return common.NewMessage(reason)
		}
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
	if err := ValidateUpstreamCostSettings(&channel, next); err != nil {
		return err
	}
	sameRatio := old.UpstreamCostRatio == nil && next.UpstreamCostRatio == nil ||
		old.UpstreamCostRatio != nil && next.UpstreamCostRatio != nil && *old.UpstreamCostRatio == *next.UpstreamCostRatio
	if sameRatio && old.UpstreamCostCurrency == next.UpstreamCostCurrency {
		return nil
	}
	oldCurrency := old.UpstreamCostCurrency
	if oldCurrency == "" {
		oldCurrency = currentChannelCostCurrency()
	}
	currency := next.UpstreamCostCurrency
	if currency == "" {
		currency = currentChannelCostCurrency()
	}
	return tx.Create(&ChannelCostRatioHistory{
		ChannelID: channel.Id, OldRatio: old.UpstreamCostRatio, NewRatio: next.UpstreamCostRatio,
		OldCurrency: oldCurrency,
		Currency:    currency, Source: source, OperatorID: channel.CostRatioOperatorID,
		UpstreamInfo: upstreamInfo, CreatedAt: common.GetTimestamp(),
	}).Error
}

func currentChannelCostCurrency() string {
	currency := operation_setting.GetQuotaDisplayType()
	if currency == operation_setting.QuotaDisplayTypeTokens {
		return operation_setting.CurrencyUSD
	}
	return currency
}

// UpdateChannelCostRatioSyncSettings changes only the sync-related setting
// fields and records an operator history entry when the currency changes.
func UpdateChannelCostRatioSyncSettings(channelID, operatorID int, currency string, minRatio, maxRatio *float64, enabled bool) error {
	var channel Channel
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&channel, channelID).Error; err != nil {
			return err
		}
		if eligible, reason := UpstreamRatioSyncEligibility(&channel); !eligible {
			return common.NewMessage(reason)
		}
		old := channel.GetSetting()
		next := old
		next.UpstreamCostCurrency = currency
		next.UpstreamCostRatioSyncEnabled = enabled
		next.UpstreamCostRatioSyncMin = minRatio
		next.UpstreamCostRatioSyncMax = maxRatio
		if err := ValidateUpstreamCostSettings(&channel, next); err != nil {
			return err
		}
		raw := "{}"
		if channel.Setting != nil && *channel.Setting != "" {
			raw = *channel.Setting
		}
		var err error
		for _, field := range []struct {
			path  string
			value any
		}{
			{"upstream_cost_currency", currency},
			{"upstream_cost_ratio_sync_enabled", enabled},
			{"upstream_cost_ratio_sync_min", minRatio},
			{"upstream_cost_ratio_sync_max", maxRatio},
		} {
			if field.path == "upstream_cost_currency" && currency == "" ||
				(field.path == "upstream_cost_ratio_sync_min" && minRatio == nil) ||
				(field.path == "upstream_cost_ratio_sync_max" && maxRatio == nil) ||
				(field.path == "upstream_cost_ratio_sync_enabled" && !enabled) {
				raw, err = sjson.Delete(raw, field.path)
			} else {
				raw, err = sjson.Set(raw, field.path, field.value)
			}
			if err != nil {
				return err
			}
		}
		channel.Setting = &raw
		if err := tx.Model(&Channel{}).Where("id = ?", channelID).UpdateColumn("setting", raw).Error; err != nil {
			return err
		}
		if old.UpstreamCostCurrency != next.UpstreamCostCurrency {
			channel.CostRatioOperatorID = operatorID
			if err := recordChannelCostRatioChange(tx, old, channel, "manual", ""); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		CacheUpdateChannel(&channel)
	}
	return err
}

// DisableChannelCostRatioSync removes only the persisted opt-in flag.
func DisableChannelCostRatioSync(channelID int) (bool, error) {
	var channel Channel
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&channel, channelID).Error; err != nil {
			return err
		}
		if channel.Setting == nil || *channel.Setting == "" {
			return nil
		}
		raw, err := sjson.Delete(*channel.Setting, "upstream_cost_ratio_sync_enabled")
		if err != nil {
			return err
		}
		channel.Setting = &raw
		return tx.Model(&Channel{}).Where("id = ?", channelID).UpdateColumn("setting", raw).Error
	})
	if err == nil {
		CacheUpdateChannel(&channel)
	}
	return err == nil, err
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
// channel edits, rejecting results for an upstream identity that changed
// during the network request.
func ApplyChannelCostRatioSync(probed *Channel, value *float64, status, message, upstreamInfo string, automatic bool) (result ChannelCostRatioSyncResult, err error) {
	if probed == nil {
		return result, common.NewMessage("Channel configuration changed during upstream ratio sync")
	}
	return applyChannelCostRatioSync(probed, probed.Id, value, status, message, upstreamInfo, automatic)
}

func applyChannelCostRatioSync(probed *Channel, channelID int, value *float64, status, message, upstreamInfo string, automatic bool) (result ChannelCostRatioSyncResult, err error) {
	var channel Channel
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&channel, channelID).Error; err != nil {
			return err
		}
		if eligible, reason := UpstreamRatioSyncEligibility(&channel); !eligible {
			return common.NewMessage(reason)
		}
		settings := channel.GetSetting()
		result = ChannelCostRatioSyncResult{Status: status, OldRatio: settings.UpstreamCostRatio, NewRatio: settings.UpstreamCostRatio, Value: value, Error: message}
		if probed != nil && (channel.Type != probed.Type || channel.Key != probed.Key || channel.GetBaseURL() != probed.GetBaseURL() || channel.ChannelInfo.IsMultiKey != probed.ChannelInfo.IsMultiKey) {
			result.Status = "stale"
			result.Error = "Channel configuration changed during upstream ratio sync"
			return nil
		}
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
