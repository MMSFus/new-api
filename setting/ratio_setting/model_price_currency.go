package ratio_setting

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
)

// ModelPriceCurrencyOption stores model name -> currency code. A missing entry
// means USD. Only CNY and CUSTOM are stored.
const ModelPriceCurrencyOption = "ModelPriceCurrency"

// ModelPriceCurrencyOptionSyncField is the ratio_config / pricing sync field
// carrying the same map.
const ModelPriceCurrencyOptionSyncField = "model_price_currency"

var modelPriceCurrencyMap = types.NewRWMap[string, string]()

func ModelPriceCurrency2JSONString() string {
	return modelPriceCurrencyMap.MarshalJSONString()
}

// UpdateModelPriceCurrencyByJSONString loads the stored map. Saves are
// validated by the model pricing path; loading must not discard the whole map
// on one bad entry, since that would bill every marked model as USD. USD and
// empty entries are dropped; an unknown code is kept so billing fails closed.
func UpdateModelPriceCurrencyByJSONString(jsonStr string) error {
	var entries map[string]string
	if err := common.UnmarshalJsonStr(jsonStr, &entries); err != nil {
		return err
	}
	for name, code := range entries {
		if code == "" || code == operation_setting.CurrencyUSD {
			delete(entries, name)
			continue
		}
		if err := operation_setting.ValidateCurrency(code); err != nil {
			common.SysError(common.LogText("model %s has an unsupported price currency %s; its requests will fail until it is fixed", name, code))
		}
	}
	filtered, err := common.Marshal(entries)
	if err != nil {
		return err
	}
	// One locked swap: readers never observe an empty map mid-reload.
	return types.LoadFromJsonStringWithCallback(modelPriceCurrencyMap, string(filtered), InvalidateExposedDataCache)
}

// ValidateModelPriceCurrencyJSON accepts only CNY and CUSTOM values; USD is
// represented by the absence of an entry.
func ValidateModelPriceCurrencyJSON(jsonStr string) error {
	var entries map[string]string
	if err := common.UnmarshalJsonStr(jsonStr, &entries); err != nil {
		return err
	}
	for name, code := range entries {
		if err := operation_setting.ValidateCurrency(code); err != nil {
			return fmt.Errorf("model %s: %w", name, err)
		}
	}
	return nil
}

// GetModelPriceCurrency returns the configured currency of a model's legacy
// ratio/price and configured billing expression, applying the same name
// normalization as the price lookups. Missing entries are USD.
func GetModelPriceCurrency(name string) string {
	if code, ok := modelPriceCurrencyMap.Get(FormatMatchingModelName(name)); ok {
		return code
	}
	return operation_setting.CurrencyUSD
}

func GetModelPriceCurrencyCopy() map[string]string {
	return modelPriceCurrencyMap.ReadAll()
}

// UsedPriceCurrencies lists non-USD currencies referenced by any model price.
func UsedPriceCurrencies() map[string]bool {
	used := make(map[string]bool)
	for _, code := range modelPriceCurrencyMap.ReadAll() {
		used[code] = true
	}
	return used
}

// ResolveModelPriceCurrency returns the currency of a model's legacy price or
// ratio and the factor converting it to USD. It fails closed: an invalid
// exchange rate is an error, never a factor of 1.
func ResolveModelPriceCurrency(name string) (string, float64, error) {
	code := GetModelPriceCurrency(name)
	factor, err := operation_setting.CurrencyUSDFactor(code)
	if err != nil {
		return code, 0, fmt.Errorf("model %s: %w", name, err)
	}
	return code, factor, nil
}
