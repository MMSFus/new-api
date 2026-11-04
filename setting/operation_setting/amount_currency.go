package operation_setting

import (
	"fmt"
	"math"
	"strconv"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// Amount fields whose stored value may carry a currency mark through
// quota_setting.amount_currency. Unmarked fields keep their original unit:
// the three reward options are quota units, trust_quota_usd is USD.
const (
	AmountFieldQuotaForNewUser = "QuotaForNewUser"
	AmountFieldQuotaForInviter = "QuotaForInviter"
	AmountFieldQuotaForInvitee = "QuotaForInvitee"
	AmountFieldTrustQuotaUSD   = "trust_quota_usd"
	AmountCurrencyOptionKey    = "quota_setting.amount_currency"
)

var amountFields = []string{AmountFieldQuotaForNewUser, AmountFieldQuotaForInviter, AmountFieldQuotaForInvitee, AmountFieldTrustQuotaUSD}

// The reward options are parsed with Atoi into common.*; a currency amount
// such as 0.5 CNY needs its decimal value, so the raw number is kept here.
var (
	rewardAmountMu  sync.RWMutex
	rewardAmountRaw = map[string]float64{}
)

func IsAmountField(field string) bool {
	for _, candidate := range amountFields {
		if candidate == field {
			return true
		}
	}
	return false
}

// IsAmountOption identifies options that must share a publication boundary.
func IsAmountOption(key string) bool {
	return key == AmountCurrencyOptionKey || key == "quota_setting."+AmountFieldTrustQuotaUSD || IsAmountField(key) && key != AmountFieldTrustQuotaUSD
}

// ValidateAmountOptions requires a complete set of values whenever currencies
// are submitted. A legacy single-option request cannot change the unit alone.
func ValidateAmountOptions(values map[string]string) error {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	currencies := quotaSetting.AmountCurrency
	if raw, ok := values[AmountCurrencyOptionKey]; ok {
		if err := ValidateAmountCurrencyJSON(raw); err != nil {
			return err
		}
		currencies = make(map[string]string)
		if err := common.UnmarshalJsonStr(raw, &currencies); err != nil {
			return err
		}
		for _, field := range amountFields {
			key := field
			if field == AmountFieldTrustQuotaUSD {
				key = "quota_setting." + field
			}
			if _, ok := values[key]; !ok {
				return common.NewMessage("Save amount currencies and their values together")
			}
		}
	}
	for _, field := range amountFields {
		key := field
		if field == AmountFieldTrustQuotaUSD {
			key = "quota_setting." + field
		}
		raw, ok := values[key]
		if !ok {
			continue
		}
		number, err := strconv.ParseFloat(raw, 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
			return common.NewMessage("Enter a finite, non-negative price")
		}
		if field != AmountFieldTrustQuotaUSD && currencies[field] == "" {
			quota, err := strconv.Atoi(raw)
			if err != nil || quota < 0 {
				return common.NewMessage("Quota must be a whole number")
			}
			if err := common.ValidateWalletQuota(quota); err != nil {
				return err
			}
		} else if _, err := currencyAmountToQuota(number, currencies[field]); err != nil {
			return err
		}
	}
	return nil
}

// SetRewardAmountRaw records the decimal value of a reward option on load/save.
func SetRewardAmountRaw(field, value string) {
	number, err := strconv.ParseFloat(value, 64)
	rewardAmountMu.Lock()
	defer rewardAmountMu.Unlock()
	if err != nil {
		delete(rewardAmountRaw, field)
		return
	}
	rewardAmountRaw[field] = number
}

func ValidateAmountCurrencyJSON(jsonStr string) error {
	var entries map[string]string
	if err := common.UnmarshalJsonStr(jsonStr, &entries); err != nil {
		return err
	}
	for field, code := range entries {
		if !IsAmountField(field) {
			return fmt.Errorf("unsupported amount field: %s", field)
		}
		if err := ValidateCurrency(code); err != nil {
			return fmt.Errorf("field %s: %w", field, err)
		}
		if err := ValidateCurrencyRateAvailable(code); err != nil {
			return err
		}
	}
	return nil
}

// GetAmountCurrency returns the currency of an amount field, or USD when unmarked.
func GetAmountCurrency(field string) string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return getAmountCurrency(field)
}

func getAmountCurrency(field string) string {
	if code, ok := quotaSetting.AmountCurrency[field]; ok && code != "" {
		return code
	}
	return CurrencyUSD
}

// UsedAmountCurrencies lists non-USD currencies referenced by amount fields.
func UsedAmountCurrencies() map[string]bool {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	used := make(map[string]bool)
	for _, code := range quotaSetting.AmountCurrency {
		if code != "" && code != CurrencyUSD {
			used[code] = true
		}
	}
	return used
}

// currencyAmountToQuota converts amount (in currency) to wallet quota with
// amount * factor * QuotaPerUnit, failing closed on an invalid rate.
func currencyAmountToQuota(amount float64, currency string) (int, error) {
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 {
		return 0, fmt.Errorf("amount must be a finite non-negative number")
	}
	factor, err := CurrencyUSDFactor(currency)
	if err != nil {
		return 0, err
	}
	value := decimal.NewFromFloat(amount).Mul(decimal.NewFromFloat(factor)).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	return common.WalletQuotaFromDecimalStrict(value)
}

// RewardQuota resolves QuotaForNewUser / QuotaForInviter / QuotaForInvitee.
// Unmarked fields return their quota unchanged; marked fields convert the
// stored currency amount at the current rate. Callers skip the reward on error.
func RewardQuota(field string) (int, error) {
	// The option writer publishes a whole batch under this lock. Read the
	// legacy value, decimal value and currency from the same configuration.
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	var quotaValue int
	switch field {
	case AmountFieldQuotaForNewUser:
		quotaValue = common.QuotaForNewUser
	case AmountFieldQuotaForInviter:
		quotaValue = common.QuotaForInviter
	case AmountFieldQuotaForInvitee:
		quotaValue = common.QuotaForInvitee
	default:
		return 0, fmt.Errorf("unsupported reward field: %s", field)
	}
	currency := getAmountCurrency(field)
	if currency == CurrencyUSD {
		return quotaValue, nil
	}
	rewardAmountMu.RLock()
	amount, ok := rewardAmountRaw[field]
	rewardAmountMu.RUnlock()
	if !ok {
		amount = float64(quotaValue)
	}
	quota, err := currencyAmountToQuota(amount, currency)
	if err != nil {
		return 0, fmt.Errorf("%s (%s): %w", field, currency, err)
	}
	return quota, nil
}

// TrustQuota returns the wallet trust-bypass threshold in quota units. An
// invalid exchange rate disables the bypass (returns 0).
func TrustQuota() float64 {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	amount := quotaSetting.TrustQuotaUSD
	currency := getAmountCurrency(AmountFieldTrustQuotaUSD)
	if currency == CurrencyUSD {
		return amount * common.QuotaPerUnit
	}
	quota, err := currencyAmountToQuota(amount, currency)
	if err != nil {
		common.SysError(common.LogText("trust quota disabled: %s", err.Error()))
		return 0
	}
	return float64(quota)
}
