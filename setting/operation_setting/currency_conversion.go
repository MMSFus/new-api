package operation_setting

import (
	"fmt"
	"math"
	"strconv"

	"github.com/QuantumNous/new-api/common"
)

// Supported currency codes
const (
	CurrencyUSD    = "USD"
	CurrencyCNY    = "CNY"
	CurrencyCustom = "CUSTOM"
)

// ValidateCurrency checks if the currency code is one of the supported values.
func ValidateCurrency(code string) error {
	if code != CurrencyCNY && code != CurrencyCustom {
		return fmt.Errorf("currency must be CNY or CUSTOM, got %s", code)
	}
	return nil
}

// CurrencyUSDFactor returns how many USD equals 1 unit of the given currency.
// Returns an error if the exchange rate or its reciprocal is not finite and positive.
//
// Examples:
//   - USD: returns 1.0 (1 USD = 1 USD)
//   - CNY with USDExchangeRate=7.3: returns 1/7.3 ≈ 0.137 (1 CNY = 0.137 USD)
//   - CUSTOM with rate=2.0: returns 1/2.0 = 0.5 (1 CUSTOM = 0.5 USD)
func CurrencyUSDFactor(code string) (float64, error) {
	if code == "" || code == CurrencyUSD {
		return 1.0, nil
	}

	var rate float64
	switch code {
	case CurrencyCNY:
		rate = USDExchangeRate
	case CurrencyCustom:
		rate = GetGeneralSetting().CustomCurrencyExchangeRate
	default:
		return 0, fmt.Errorf("unsupported currency: %s", code)
	}

	if !validRate(rate) {
		return 0, fmt.Errorf("invalid exchange rate for %s: %v (must be positive and finite)", code, rate)
	}

	factor := 1 / rate
	if !validRate(factor) {
		return 0, fmt.Errorf("invalid exchange rate for %s: reciprocal is not finite and positive", code)
	}
	return factor, nil
}

// Option keys holding the exchange rates behind CNY and CUSTOM.
const (
	USDExchangeRateOptionKey          = "USDExchangeRate"
	CustomCurrencyRateOptionKey       = "general_setting.custom_currency_exchange_rate"
	ErrCurrencyRateInUseMessage       = "Exchange rate must be a finite positive number while prices or amounts use this currency"
	ErrCurrencyRateUnavailableMessage = "The selected currency has no valid exchange rate"
)

func validRate(rate float64) bool {
	return rate > 0 && !math.IsNaN(rate) && !math.IsInf(rate, 0)
}

func validCurrencyRate(rate float64) bool {
	return validRate(rate) && validRate(1/rate)
}

// ValidateExchangeRateOption rejects saving an invalid rate for a currency
// that is referenced by a model price or amount field (used).
func ValidateExchangeRateOption(key, value string, used map[string]bool) error {
	var currency string
	switch key {
	case USDExchangeRateOptionKey:
		currency = CurrencyCNY
	case CustomCurrencyRateOptionKey:
		currency = CurrencyCustom
	default:
		return nil
	}
	if !used[currency] {
		return nil
	}
	rate, err := strconv.ParseFloat(value, 64)
	if err != nil || !validCurrencyRate(rate) {
		return common.NewMessage(ErrCurrencyRateInUseMessage)
	}
	return nil
}

// ValidateCurrencyRateAvailable rejects marking a price or amount with a
// currency whose current exchange rate is invalid.
func ValidateCurrencyRateAvailable(code string) error {
	if code == "" || code == CurrencyUSD {
		return nil
	}
	if _, err := CurrencyUSDFactor(code); err != nil {
		return common.NewMessage(ErrCurrencyRateUnavailableMessage)
	}
	return nil
}
