package operation_setting

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCurrency(t *testing.T) {
	tests := []struct {
		code      string
		wantError bool
	}{
		{CurrencyCNY, false},
		{CurrencyCustom, false},
		{CurrencyUSD, true},
		{"", true},
		{"EUR", true},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			err := ValidateCurrency(tt.code)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCurrencyUSDFactor(t *testing.T) {
	// Save original values
	origUSD := USDExchangeRate
	origGeneral := generalSetting
	defer func() {
		USDExchangeRate = origUSD
		generalSetting = origGeneral
	}()

	tests := []struct {
		name         string
		code         string
		setupRate    func()
		wantFactor   float64
		wantError    bool
		errorMessage string
	}{
		{
			name:       "USD returns 1",
			code:       CurrencyUSD,
			setupRate:  func() {},
			wantFactor: 1.0,
			wantError:  false,
		},
		{
			name:       "empty string defaults to USD",
			code:       "",
			setupRate:  func() {},
			wantFactor: 1.0,
			wantError:  false,
		},
		{
			name: "CNY with valid rate",
			code: CurrencyCNY,
			setupRate: func() {
				USDExchangeRate = 7.3
			},
			wantFactor: 1.0 / 7.3,
			wantError:  false,
		},
		{
			name: "CUSTOM with valid rate",
			code: CurrencyCustom,
			setupRate: func() {
				generalSetting.CustomCurrencyExchangeRate = 2.0
			},
			wantFactor: 0.5,
			wantError:  false,
		},
		{
			name: "CNY with zero rate fails",
			code: CurrencyCNY,
			setupRate: func() {
				USDExchangeRate = 0
			},
			wantError:    true,
			errorMessage: "invalid exchange rate",
		},
		{
			name: "CNY with negative rate fails",
			code: CurrencyCNY,
			setupRate: func() {
				USDExchangeRate = -7.3
			},
			wantError:    true,
			errorMessage: "invalid exchange rate",
		},
		{
			name: "CNY with NaN fails",
			code: CurrencyCNY,
			setupRate: func() {
				USDExchangeRate = math.NaN()
			},
			wantError:    true,
			errorMessage: "invalid exchange rate",
		},
		{
			name: "CNY with +Inf fails",
			code: CurrencyCNY,
			setupRate: func() {
				USDExchangeRate = math.Inf(1)
			},
			wantError:    true,
			errorMessage: "invalid exchange rate",
		},
		{
			name: "CUSTOM with zero rate fails",
			code: CurrencyCustom,
			setupRate: func() {
				generalSetting.CustomCurrencyExchangeRate = 0
			},
			wantError:    true,
			errorMessage: "invalid exchange rate",
		},
		{
			name:         "unsupported currency fails",
			code:         "EUR",
			setupRate:    func() {},
			wantError:    true,
			errorMessage: "unsupported currency",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupRate()

			factor, err := CurrencyUSDFactor(tt.code)

			if tt.wantError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMessage)
			} else {
				require.NoError(t, err)
				assert.InDelta(t, tt.wantFactor, factor, 1e-9)
			}
		})
	}
}

func withAmountCurrencies(t *testing.T, cny, custom float64, marks map[string]string) {
	t.Helper()
	oldCNY, oldGeneral, oldQuota := USDExchangeRate, generalSetting, quotaSetting
	rewardAmountMu.Lock()
	oldRaw := rewardAmountRaw
	rewardAmountRaw = map[string]float64{}
	rewardAmountMu.Unlock()
	t.Cleanup(func() {
		USDExchangeRate, generalSetting, quotaSetting = oldCNY, oldGeneral, oldQuota
		rewardAmountMu.Lock()
		rewardAmountRaw = oldRaw
		rewardAmountMu.Unlock()
	})
	USDExchangeRate = cny
	generalSetting.CustomCurrencyExchangeRate = custom
	quotaSetting.AmountCurrency = marks
}

// Unmarked reward fields keep their quota value; marked ones store a currency
// amount converted with amount * factor * QuotaPerUnit at the current rate.
func TestRewardQuotaAndTrustQuota(t *testing.T) {
	withAmountCurrencies(t, 8, 2, map[string]string{
		AmountFieldQuotaForInviter: CurrencyCNY,
		AmountFieldQuotaForInvitee: CurrencyCustom,
		AmountFieldTrustQuotaUSD:   CurrencyCNY,
	})
	SetRewardAmountRaw(AmountFieldQuotaForInviter, "4")
	SetRewardAmountRaw(AmountFieldQuotaForInvitee, "0.5")
	quotaSetting.TrustQuotaUSD = 80

	tests := []struct {
		field string
		value int
		want  int
	}{
		{AmountFieldQuotaForNewUser, 123456, 123456}, // unmarked: unchanged
		{AmountFieldQuotaForInviter, 4, 250000},      // 4 CNY / 8 * 500000
		{AmountFieldQuotaForInvitee, 0, 125000},      // 0.5 CUSTOM / 2 * 500000
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			got, err := RewardQuota(tt.field, tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
	assert.Equal(t, float64(5_000_000), TrustQuota()) // 80 CNY / 8 * 500000

	// Invalid rate: rewards fail (callers skip them), trust bypass is disabled.
	USDExchangeRate = math.NaN()
	_, err := RewardQuota(AmountFieldQuotaForInviter, 4)
	assert.Error(t, err)
	got, err := RewardQuota(AmountFieldQuotaForNewUser, 123456)
	require.NoError(t, err)
	assert.Equal(t, 123456, got)
	assert.Equal(t, float64(0), TrustQuota())

	// Unmarked trust threshold keeps the USD formula.
	quotaSetting.AmountCurrency = map[string]string{}
	quotaSetting.TrustQuotaUSD = 10
	assert.Equal(t, 10*common.QuotaPerUnit, TrustQuota())
}

func TestCurrencyOptionValidation(t *testing.T) {
	withAmountCurrencies(t, 7.3, 0, nil)

	used := map[string]bool{CurrencyCNY: true}
	for _, value := range []string{"0", "-1", "NaN", "Inf", "abc"} {
		assert.Error(t, ValidateExchangeRateOption(USDExchangeRateOptionKey, value, used), value)
	}
	assert.NoError(t, ValidateExchangeRateOption(USDExchangeRateOptionKey, "7.1", used))
	// Unused currencies keep their previous, unrestricted behavior.
	assert.NoError(t, ValidateExchangeRateOption(CustomCurrencyRateOptionKey, "0", used))
	assert.Error(t, ValidateExchangeRateOption(CustomCurrencyRateOptionKey, "0", map[string]bool{CurrencyCustom: true}))

	assert.NoError(t, ValidateQuotaOption(AmountCurrencyOptionKey, `{"QuotaForNewUser":"CNY"}`))
	assert.Error(t, ValidateQuotaOption(AmountCurrencyOptionKey, `{"QuotaForNewUser":"USD"}`))
	assert.Error(t, ValidateQuotaOption(AmountCurrencyOptionKey, `{"PreConsumedQuota":"CNY"}`))
	// CUSTOM has rate 0 here, so marking a field with it is rejected.
	assert.Error(t, ValidateQuotaOption(AmountCurrencyOptionKey, `{"trust_quota_usd":"CUSTOM"}`))
}
