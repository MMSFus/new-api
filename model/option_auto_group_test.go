package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateOptionValueRejectsInvalidMaxTokenAutoGroups(t *testing.T) {
	for _, value := range []string{"", "0", "-1", "1.5", "invalid"} {
		t.Run(value, func(t *testing.T) {
			assert.Error(t, validateOptionValue("MaxTokenAutoGroups", value))
		})
	}
	require.NoError(t, validateOptionValue("MaxTokenAutoGroups", "999999"))
}

// A rate used by a model price cannot be saved as an invalid value, and a
// model price can only be marked with a currency whose rate is usable.
func TestCurrencyOptionAndModelPricingValidation(t *testing.T) {
	oldRate := operation_setting.USDExchangeRate
	oldCustom := operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate
	savedMarks := ratio_setting.ModelPriceCurrency2JSONString()
	t.Cleanup(func() {
		operation_setting.USDExchangeRate = oldRate
		operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate = oldCustom
		require.NoError(t, ratio_setting.UpdateModelPriceCurrencyByJSONString(savedMarks))
	})
	operation_setting.USDExchangeRate = 7.3
	operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate = 0
	require.NoError(t, ratio_setting.UpdateModelPriceCurrencyByJSONString(`{"cny-model":"CNY"}`))

	assert.Error(t, validateOptionValue("USDExchangeRate", "0"))
	assert.Error(t, validateOptionValue("USDExchangeRate", "NaN"))
	assert.NoError(t, validateOptionValue("USDExchangeRate", "7.1"))
	assert.NoError(t, validateOptionValue("general_setting.custom_currency_exchange_rate", "0"))

	tests := []struct {
		name     string
		draft    PricingValues
		previous PricingValues
		wantErr  bool
	}{
		{"CNY mark", PricingValues{"ModelPrice": 8.0, "ModelPriceCurrency": "CNY"}, PricingValues{}, false},
		{"explicit USD is the absence of a mark", PricingValues{"ModelPrice": 1.0, "ModelPriceCurrency": "USD"}, PricingValues{}, false},
		{"unknown currency", PricingValues{"ModelPrice": 1.0, "ModelPriceCurrency": "EUR"}, PricingValues{}, true},
		{"non-string currency", PricingValues{"ModelPrice": 1.0, "ModelPriceCurrency": 1.0}, PricingValues{}, true},
		{"new CUSTOM mark without a usable rate", PricingValues{"ModelPrice": 1.0, "ModelPriceCurrency": "CUSTOM"}, PricingValues{}, true},
		{"unchanged CUSTOM mark stays saveable", PricingValues{"ModelPrice": 2.0, "ModelPriceCurrency": "CUSTOM"}, PricingValues{"ModelPriceCurrency": "CUSTOM"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateModelPricing("currency-test-model", tt.draft, tt.previous)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}

	values := defaultPricingMaps()
	replaceModelPricing(values, "currency-test-model", PricingValues{"ModelPrice": 1.0, "ModelPriceCurrency": "USD"})
	assert.NotContains(t, values["ModelPriceCurrency"], "currency-test-model")
	replaceModelPricing(values, "currency-test-model", PricingValues{"ModelPrice": 8.0, "ModelPriceCurrency": "CNY"})
	assert.Equal(t, "CNY", values["ModelPriceCurrency"]["currency-test-model"])
	assert.Equal(t, "CNY", effectiveModelPricing(values, "currency-test-model")["ModelPriceCurrency"])
	// Overwriting the price without a mark (e.g. an upstream USD sync) clears it.
	replaceModelPricing(values, "currency-test-model", PricingValues{"ModelPrice": 1.0})
	assert.NotContains(t, values["ModelPriceCurrency"], "currency-test-model")
}
