package billing_setting

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSmokeTestTaskExprValidatesDeclaredUsageVectors(t *testing.T) {
	videoSchema := map[string]jsplugin.UsageFieldSchema{
		"seconds": {Type: "number", Unit: "second"},
		"mode":    {Enum: []string{"std", "pro"}},
		"quality": {Enum: []string{"sd", "hd"}},
	}

	tests := []struct {
		name          string
		schema        map[string]jsplugin.UsageFieldSchema
		expression    string
		expectedError string
	}{
		{
			name:          "fixed prices are not task usage prices",
			schema:        videoSchema,
			expression:    `true ? tier("normal", u("seconds") * 0.4) : tier("fixed", fixed(0.01))`,
			expectedError: "fixed pricing is not supported for task usage expressions",
		},
		{
			name:       "declared numeric and enum facts",
			schema:     videoSchema,
			expression: `u("mode") == "pro" ? tier("pro", u("seconds") * 0.8) : tier("std", u("seconds") * 0.4)`,
		},
		{
			name:          "undeclared literal key",
			schema:        videoSchema,
			expression:    `tier("base", u("clips") * 0.1)`,
			expectedError: `usage key "clips" is not declared`,
		},
		{
			name:          "negative duration boundary",
			schema:        videoSchema,
			expression:    fmt.Sprintf(`u("seconds") == %d ? -1 : 0`, relaycommon.MaxTaskDurationSeconds),
			expectedError: "result must be finite and non-negative",
		},
		{
			name:          "negative count boundary",
			schema:        map[string]jsplugin.UsageFieldSchema{"clips": {Type: "number", Unit: "count"}},
			expression:    fmt.Sprintf(`u("clips") == %d ? -1 : 0`, dto.MaxImageN),
			expectedError: "result must be finite and non-negative",
		},
		{
			name:          "negative token boundary",
			schema:        map[string]jsplugin.UsageFieldSchema{"tokens": {Type: "number", Unit: "token"}},
			expression:    fmt.Sprintf(`u("tokens") == %d ? -1 : 0`, common.MaxQuota),
			expectedError: "result must be finite and non-negative",
		},
		{
			name:          "negative credit boundary",
			schema:        map[string]jsplugin.UsageFieldSchema{"units": {Type: "number", Unit: "credit"}},
			expression:    fmt.Sprintf(`u("units") == %d ? -1 : 0`, common.MaxQuota),
			expectedError: "result must be finite and non-negative",
		},
		{
			name:          "negative enum combination",
			schema:        videoSchema,
			expression:    `u("mode") == "pro" && u("quality") == "hd" ? -1 : 0`,
			expectedError: "result must be finite and non-negative",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			err := SmokeTestTaskExpr(testCase.expression, testCase.schema)
			if testCase.expectedError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, testCase.expectedError)
		})
	}
}

func TestSmokeTestTaskExprCapsOversizedEnumProductsAtLastCombination(t *testing.T) {
	schema := make(map[string]jsplugin.UsageFieldSchema, 7)
	condition := ""
	for index := range 7 {
		schema[fmt.Sprintf("enum_%d", index)] = jsplugin.UsageFieldSchema{Enum: []string{"first", "middle", "last"}}
		if condition != "" {
			condition += " && "
		}
		condition += fmt.Sprintf(`u("enum_%d") == "last"`, index)
	}

	err := SmokeTestTaskExpr(condition+" ? -1 : 0", schema)
	require.ErrorContains(t, err, "result must be finite and non-negative")
}

func TestSmokeTestExprRejectsTaskUsageWithoutSchema(t *testing.T) {
	err := SmokeTestExpr(`u("mode") == "std" ? 1 : 2`)
	require.Error(t, err)
	assert.ErrorContains(t, err, "mode")
	assert.ErrorContains(t, err, "no task plugin usage schema")

	require.NoError(t, SmokeTestExpr(`tier("base", p * 2 + c * 8)`))
}

// A configured expression and its plugin overrides follow the model's
// currency mark; a built-in expression is always USD.
func TestResolveBillingExprCurrencyOwner(t *testing.T) {
	oldRate := operation_setting.USDExchangeRate
	savedMarks := ratio_setting.ModelPriceCurrency2JSONString()
	saved := billingSetting
	t.Cleanup(func() {
		operation_setting.USDExchangeRate = oldRate
		require.NoError(t, ratio_setting.UpdateModelPriceCurrencyByJSONString(savedMarks))
		billingSetting = saved
	})
	operation_setting.USDExchangeRate = 8
	builtin := ""
	for name := range builtinBillingExpr {
		builtin = name
		break
	}
	require.NotEmpty(t, builtin)
	require.NoError(t, ratio_setting.UpdateModelPriceCurrencyByJSONString(fmt.Sprintf(`{"cny-model":"CNY",%q:"CNY"}`, builtin)))
	billingSetting = BillingSetting{
		BillingMode:       map[string]string{"cny-model": BillingModeTieredExpr},
		BillingExpr:       map[string]string{"cny-model": `tier("base", u("seconds") * 1)`},
		PluginBillingExpr: map[string]string{PluginBillingExprKey("demo", "cny-model"): `tier("base", u("seconds") * 2)`},
	}

	code, factor, err := ResolveBillingExprCurrency("cny-model")
	require.NoError(t, err)
	assert.Equal(t, "CNY", code)
	assert.Equal(t, 0.125, factor)

	if GetBillingMode(builtin) == BillingModeTieredExpr {
		code, factor, err = ResolveBillingExprCurrency(builtin)
		require.NoError(t, err)
		assert.Equal(t, "USD", code)
		assert.Equal(t, 1.0, factor)
	}

	expression, code, factor, ok, err := ResolveTaskBillingExprWithCurrency("demo", "cny-model", "")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, `tier("base", u("seconds") * 2)`, expression)
	assert.Equal(t, "CNY", code)
	assert.Equal(t, 0.125, factor)

	operation_setting.USDExchangeRate = 0
	_, _, _, ok, err = ResolveTaskBillingExprWithCurrency("demo", "cny-model", "")
	assert.True(t, ok)
	assert.Error(t, err)
}
