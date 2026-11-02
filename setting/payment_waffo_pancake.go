package setting

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/shopspring/decimal"
	"golang.org/x/text/currency"
)

// Waffo Pancake hosted checkout configuration. Gateway is enabled once
// MerchantID + PrivateKey + ProductID are populated (no separate Enabled
// flag, matching Stripe / Creem). StoreID + ProductID are operator-bound
// via SaveWaffoPancakeConfig.
//
// WaffoPancakeCurrency is the ISO 4217 settlement currency used for checkout
// sessions and product prices. WaffoPancakeUnitPrice is the amount in that
// currency charged per unit of top-up quota.
var (
	WaffoPancakeMerchantID string
	WaffoPancakePrivateKey string
	WaffoPancakeReturnURL  string
	WaffoPancakeCurrency           = DefaultWaffoPancakeCurrency
	WaffoPancakeUnitPrice  float64 = 1.0
	WaffoPancakeMinTopUp   int     = 1
	WaffoPancakeStoreID    string
	WaffoPancakeProductID  string
)

const DefaultWaffoPancakeCurrency = "USD"

// Same shape the Pancake SDK enforces (validateCurrencyCode).
var waffoPancakeCurrencyRe = regexp.MustCompile(`^[A-Z]{3}$`)

// ValidateWaffoPancakeCurrency accepts only three uppercase letters that name
// a real ISO 4217 currency. "XXX" (no currency) is rejected.
func ValidateWaffoPancakeCurrency(code string) error {
	if !waffoPancakeCurrencyRe.MatchString(code) {
		return fmt.Errorf("currency must be a 3-letter uppercase ISO 4217 code, got %q", code)
	}
	unit, err := currency.ParseISO(code)
	if err != nil || unit == currency.XXX {
		return fmt.Errorf("unknown ISO 4217 currency %q", code)
	}
	return nil
}

// GetWaffoPancakeCurrency returns the configured settlement currency, or USD
// when unset (older installs that never saved the option).
func GetWaffoPancakeCurrency() string {
	if code := strings.TrimSpace(WaffoPancakeCurrency); code != "" {
		return code
	}
	return DefaultWaffoPancakeCurrency
}

// WaffoPancakeCurrencyScale returns the ISO 4217 minor-unit digits of code
// (USD 2, JPY/KRW 0, BHD 3). Unknown codes fall back to 2.
func WaffoPancakeCurrencyScale(code string) int32 {
	unit, err := currency.ParseISO(code)
	if err != nil {
		return 2
	}
	scale, _ := currency.Standard.Rounding(unit)
	return int32(scale)
}

// FormatWaffoPancakeAmount renders amount in the display format Pancake
// expects (SDK validateAmountString: ^\d+(\.\d+)?$), rounded half away from
// zero to the minor-unit digits of code: "9.99" for USD, "1000" for JPY.
func FormatWaffoPancakeAmount(amount decimal.Decimal, code string) string {
	return amount.StringFixed(WaffoPancakeCurrencyScale(code))
}

// WaffoPancakeMinChargeAmount is one minor unit of code (0.01 USD, 1 JPY),
// the smallest amount a checkout can carry after formatting.
func WaffoPancakeMinChargeAmount(code string) decimal.Decimal {
	return decimal.New(1, -WaffoPancakeCurrencyScale(code))
}
