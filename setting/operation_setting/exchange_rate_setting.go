package operation_setting

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

// ExchangeRateSetting controls automatic synchronization of USDExchangeRate.
// LastSyncedAt, LastError and LastSource are written only by the sync service.
type ExchangeRateSetting struct {
	AutoSyncEnabled   bool   `json:"auto_sync_enabled"`
	SyncIntervalHours int    `json:"sync_interval_hours"`
	LastSyncedAt      int64  `json:"last_synced_at"` // Unix seconds of the last attempt
	LastError         string `json:"last_error"`     // empty when the last attempt succeeded
	LastSource        string `json:"last_source"`
}

const (
	ExchangeRateSettingName              = "exchange_rate_setting"
	ExchangeRateSyncIntervalOptionKey    = ExchangeRateSettingName + ".sync_interval_hours"
	DefaultExchangeRateSyncIntervalHours = 6
	MinExchangeRateSyncIntervalHours     = 1
	MaxExchangeRateSyncIntervalHours     = 168
)

var exchangeRateSetting = ExchangeRateSetting{
	AutoSyncEnabled:   false,
	SyncIntervalHours: DefaultExchangeRateSyncIntervalHours,
}

func init() {
	config.GlobalConfig.Register(ExchangeRateSettingName, &exchangeRateSetting)
}

func GetExchangeRateSetting() *ExchangeRateSetting {
	return &exchangeRateSetting
}

// IsExchangeRateSyncStatusOption reports keys that only the sync service may write.
func IsExchangeRateSyncStatusOption(key string) bool {
	switch key {
	case ExchangeRateSettingName + ".last_synced_at",
		ExchangeRateSettingName + ".last_error",
		ExchangeRateSettingName + ".last_source":
		return true
	}
	return false
}

func ValidateExchangeRateSyncInterval(value string) error {
	hours, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || hours < MinExchangeRateSyncIntervalHours || hours > MaxExchangeRateSyncIntervalHours {
		return common.NewMessage("Exchange rate sync interval must be between {{min}} and {{max}} hours", map[string]any{
			"min": MinExchangeRateSyncIntervalHours,
			"max": MaxExchangeRateSyncIntervalHours,
		})
	}
	return nil
}

// NormalizeExchangeRateSyncInterval guards the scheduler against values stored
// before validation existed.
func NormalizeExchangeRateSyncInterval(hours int) int {
	if hours < MinExchangeRateSyncIntervalHours || hours > MaxExchangeRateSyncIntervalHours {
		return DefaultExchangeRateSyncIntervalHours
	}
	return hours
}
