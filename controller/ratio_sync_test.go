package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPricingSyncExpressionPriority(t *testing.T) {
	expression := `tier("base", p * 2 + c * 8 + cr * 0)`
	cases := []struct {
		name       string
		local      map[string]any
		source     map[string]any
		wantFields []string
	}{
		{"equal expressions suppress stale ratios", map[string]any{"billing_mode": map[string]string{"m": "tiered_expr"}, "billing_expr": map[string]string{"m": expression}}, map[string]any{"billing_mode": map[string]string{"m": "tiered_expr"}, "billing_expr": map[string]string{"m": expression}, "model_ratio": map[string]float64{"m": 3}, "model_price": map[string]float64{"m": 2}}, nil},
		{"local expression excludes legacy-only source", map[string]any{"billing_mode": map[string]string{"m": "tiered_expr"}, "billing_expr": map[string]string{"m": expression}}, map[string]any{"model_ratio": map[string]float64{"m": 3}, "completion_ratio": map[string]float64{"m": 2}}, nil},
		{"expression imports without legacy conflicts", map[string]any{"model_ratio": map[string]float64{"m": 1}}, map[string]any{"billing_mode": map[string]string{"m": "tiered_expr"}, "billing_expr": map[string]string{"m": expression}, "model_ratio": map[string]float64{"m": 3}, "model_price": map[string]float64{"m": 2}}, []string{"billing_mode", "billing_expr"}},
		{"inactive expression follows explicit ratio mode", map[string]any{"model_ratio": map[string]float64{"m": 1}}, map[string]any{"billing_mode": map[string]string{"m": "ratio"}, "billing_expr": map[string]string{"m": expression}, "model_ratio": map[string]float64{"m": 3}}, []string{"model_ratio"}},
		{"empty active expression never imports a false free price", map[string]any{}, map[string]any{"billing_mode": map[string]string{"m": "tiered_expr"}, "billing_expr": map[string]string{"m": " "}, "model_ratio": map[string]float64{"m": 0}}, nil},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			diff := buildDifferences(tt.local, []struct {
				name string
				data map[string]any
			}{{"source", tt.source}})
			fields := make([]string, 0, len(diff["m"]))
			for field := range diff["m"] {
				fields = append(fields, field)
			}
			assert.ElementsMatch(t, tt.wantFields, fields)
		})
	}
}

func TestRatioConfigExportsEffectiveExpressions(t *testing.T) {
	before := config.GlobalConfig.ExportAllConfigs()
	expose := ratio_setting.IsExposeRatioEnabled()
	t.Cleanup(func() {
		config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"billing_mode": before["billing_setting.billing_mode"], "billing_expr": before["billing_setting.billing_expr"]})
		ratio_setting.SetExposeRatioEnabled(expose)
	})
	config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"billing_mode": `{"sync-export":"tiered_expr"}`, "billing_expr": `{"sync-export":"tier(\"base\", p * 2)"}`})
	ratio_setting.SetExposeRatioEnabled(true)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/ratio_config", nil)
	GetRatioConfig(c)
	var response struct {
		Success bool
		Data    map[string]any
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.Equal(t, billing_setting.BillingModeTieredExpr, valueMap(response.Data["billing_mode"])["sync-export"])
	assert.Equal(t, `tier("base", p * 2)`, valueMap(response.Data["billing_expr"])["sync-export"])
}

func TestPricingSyncCompleteSourcesAndArrayFormats(t *testing.T) {
	before := config.GlobalConfig.ExportAllConfigs()
	oldRatios, oldCompletion := ratio_setting.ModelRatio2JSONString(), ratio_setting.CompletionRatio2JSONString()
	t.Cleanup(func() {
		config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"billing_mode": before["billing_setting.billing_mode"], "billing_expr": before["billing_setting.billing_expr"]})
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletion))
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"sync-token":1}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"sync-token":2}`))
	expression := `len <= 200000 ? tier("short", p * 2 + c * 8 + cr * 0) : tier("long", p * 4 + c * 12)`
	expressions, err := common.Marshal(map[string]string{"sync-already": expression})
	require.NoError(t, err)
	config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"billing_mode": `{"sync-already":"tiered_expr"}`, "billing_expr": string(expressions)})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data any
		if r.URL.Path == "/ratio_config" {
			data = map[string]any{
				"billing_mode":     map[string]string{"sync-already": "tiered_expr", "sync-expression": "tiered_expr"},
				"billing_expr":     map[string]string{"sync-already": expression, "sync-expression": expression},
				"model_ratio":      map[string]float64{"sync-already": 9, "sync-expression": 9, "sync-token": 1},
				"model_price":      map[string]float64{"sync-expression": 4},
				"completion_ratio": map[string]float64{"sync-token": 4},
				"cache_ratio":      map[string]float64{"sync-token": 0},
			}
		} else {
			data = []map[string]any{
				{"model_name": "sync-already", "model_ratio": 5, "completion_ratio": 3},
				{"model_name": "sync-expression", "model_ratio": 2, "model_price": 1},
				{"model_name": "sync-array-expression", "billing_mode": "tiered_expr", "billing_expr": expression, "quota_type": 1, "model_price": 0},
				{"model_name": "sync-unpriced"},
				{"model_name": "sync-invalid-expression", "billing_mode": "tiered_expr", "billing_expr": "", "model_ratio": 0},
				{"model_name": "sync-free", "model_ratio": 0, "completion_ratio": 0},
			}
		}
		encoded, err := common.Marshal(map[string]any{"success": true, "data": data})
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(encoded)
	}))
	defer server.Close()
	var response struct {
		Success bool
		Data    struct {
			Differences map[string]map[string]dto.DifferenceItem
			Prices      map[string]struct {
				Current   map[string]any
				Upstreams map[string]map[string]any
			}
			TestResults []dto.TestResult `json:"test_results"`
		}
	}
	body := map[string]any{"upstreams": []map[string]any{
		{"id": 1, "name": "Expressions", "base_url": server.URL, "endpoint": "/ratio_config"},
		{"id": 2, "name": "Legacy", "base_url": server.URL, "endpoint": "/pricing"},
	}}
	recorder := modelManagementRequest(t, FetchUpstreamRatios, http.MethodPost, "/api/channel/fetch_upstream_ratios", body, &response)
	require.True(t, response.Success, recorder.Body.String())
	require.Len(t, response.Data.TestResults, 2)
	for _, result := range response.Data.TestResults {
		require.Equal(t, "success", result.Status, result.Error)
	}
	assert.NotContains(t, response.Data.Differences, "sync-already")
	assert.NotContains(t, response.Data.Differences, "sync-unpriced")
	assert.NotContains(t, response.Data.Differences, "sync-invalid-expression")
	assert.Equal(t, map[string]any{"billing_mode": "tiered_expr", "billing_expr": expression}, response.Data.Prices["sync-expression"].Upstreams["Expressions(1)"])
	assert.NotContains(t, response.Data.Prices["sync-expression"].Upstreams, "Legacy(2)")
	assert.Equal(t, map[string]any{"billing_mode": "tiered_expr", "billing_expr": expression}, response.Data.Prices["sync-array-expression"].Upstreams["Legacy(2)"])
	assert.Equal(t, float64(1), response.Data.Prices["sync-token"].Upstreams["Expressions(1)"]["model_ratio"], "unchanged base prices are included for a complete price preview")
	assert.Equal(t, float64(4), response.Data.Prices["sync-token"].Upstreams["Expressions(1)"]["completion_ratio"])
	assert.Equal(t, float64(0), response.Data.Prices["sync-token"].Upstreams["Expressions(1)"]["cache_ratio"])
	assert.Equal(t, float64(0), response.Data.Prices["sync-free"].Upstreams["Legacy(2)"]["model_ratio"])
	assert.NotContains(t, response.Data.Prices, "sync-already")
	body["include_unchanged"] = true
	recorder = modelManagementRequest(t, FetchUpstreamRatios, http.MethodPost, "/api/ratio_sync/fetch", body, &response)
	require.True(t, response.Success, recorder.Body.String())
	assert.Equal(t, map[string]any{"billing_mode": "tiered_expr", "billing_expr": expression}, response.Data.Prices["sync-already"].Upstreams["Expressions(1)"])
	assert.NotContains(t, response.Data.Prices["sync-already"].Upstreams, "Legacy(2)")
	assert.NotContains(t, response.Data.Prices, "sync-unpriced")
	assert.NotContains(t, response.Data.Prices, "sync-invalid-expression")
	assert.NotContains(t, response.Data.Differences, "sync-already")
}

func TestPricingSyncForeignCurrencyExpressions(t *testing.T) {
	before := config.GlobalConfig.ExportAllConfigs()
	oldRatios, oldPrices, oldCurrencies := ratio_setting.ModelRatio2JSONString(), ratio_setting.ModelPrice2JSONString(), ratio_setting.ModelPriceCurrency2JSONString()
	t.Cleanup(func() {
		config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"billing_mode": before["billing_setting.billing_mode"], "billing_expr": before["billing_setting.billing_expr"]})
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
		require.NoError(t, ratio_setting.UpdateModelPriceCurrencyByJSONString(oldCurrencies))
	})
	config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"billing_mode": "{}", "billing_expr": "{}"})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"sync-cny-token":1,"sync-custom-fixed":1}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateModelPriceCurrencyByJSONString(`{}`))
	expression := `tier("base", p * 70)`
	cases := []struct {
		name        string
		model       string
		currencyKey string
		currency    string
		legacyField string
		mode        string
		want        map[string]any
	}{
		{name: "CNY expression with stale token ratios", model: "sync-cny-token", currency: "CNY", legacyField: "model_ratio"},
		{name: "CUSTOM expression with stale fixed price", model: "sync-custom-fixed", currency: "CUSTOM", legacyField: "model_price"},
		{name: "CNY thinking alias", model: "gemini-2.5-pro-thinking-128", currencyKey: "gemini-2.5-pro-thinking-*", currency: "CNY"},
		{name: "CUSTOM gizmo alias", model: "gpt-4-gizmo-sync", currencyKey: "gpt-4-gizmo-*", currency: "CUSTOM"},
		{name: "unsupported currency fails closed", model: "sync-unknown", currency: "EUR", legacyField: "model_ratio"},
		{name: "unmarked expression", model: "sync-usd", want: map[string]any{"billing_mode": "tiered_expr", "billing_expr": expression}},
		{name: "explicit USD expression", model: "sync-explicit-usd", currency: "USD", want: map[string]any{"billing_mode": "tiered_expr", "billing_expr": expression}},
		{name: "currency lookup uses normalized key only", model: "gemini-2.5-flash-thinking-128", currency: "CNY", want: map[string]any{"billing_mode": "tiered_expr", "billing_expr": expression}},
		{name: "inactive foreign expression keeps USD numeric export", model: "sync-inactive", currency: "CNY", legacyField: "model_ratio", mode: "ratio", want: map[string]any{"billing_mode": "ratio", "model_ratio": float64(2), "completion_ratio": float64(3)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			currencyKey := tc.currencyKey
			if currencyKey == "" {
				currencyKey = tc.model
			}
			mode := "tiered_expr"
			if tc.mode != "" {
				mode = tc.mode
			}
			data := map[string]any{
				"billing_mode":         map[string]string{tc.model: mode},
				"billing_expr":         map[string]string{tc.model: expression},
				"model_price_currency": map[string]string{currencyKey: tc.currency},
			}
			if tc.legacyField != "" {
				data[tc.legacyField] = map[string]float64{tc.model: 2}
				data["completion_ratio"] = map[string]float64{tc.model: 3}
			}
			encoded, err := common.Marshal(map[string]any{"success": true, "data": data})
			require.NoError(t, err)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(encoded)
			}))
			t.Cleanup(server.Close)
			for _, includeUnchanged := range []bool{false, true} {
				var response struct {
					Success bool
					Data    struct {
						Differences map[string]map[string]dto.DifferenceItem
						Prices      map[string]struct {
							Upstreams map[string]map[string]any
						}
						TestResults []dto.TestResult `json:"test_results"`
					}
				}
				recorder := modelManagementRequest(t, FetchUpstreamRatios, http.MethodPost, "/api/ratio_sync/fetch", map[string]any{
					"upstreams":         []map[string]any{{"name": "source", "base_url": server.URL, "endpoint": "/api/ratio_config"}},
					"include_unchanged": includeUnchanged,
				}, &response)
				require.True(t, response.Success, recorder.Body.String())
				require.Len(t, response.Data.TestResults, 1)
				require.Equal(t, "success", response.Data.TestResults[0].Status, response.Data.TestResults[0].Error)
				if tc.want == nil {
					assert.NotContains(t, response.Data.Prices, tc.model)
					assert.NotContains(t, response.Data.Differences, tc.model)
					continue
				}
				assert.Equal(t, tc.want, response.Data.Prices[tc.model].Upstreams["source"])
			}
		})
	}
}

func TestPricingSyncLocalExpressionCurrency(t *testing.T) {
	before := config.GlobalConfig.ExportAllConfigs()
	oldCurrencies := ratio_setting.ModelPriceCurrency2JSONString()
	oldRatios, oldPrices := ratio_setting.ModelRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	oldUSD, oldCustom := operation_setting.USDExchangeRate, operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate
	t.Cleanup(func() {
		config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"billing_mode": before["billing_setting.billing_mode"], "billing_expr": before["billing_setting.billing_expr"]})
		require.NoError(t, ratio_setting.UpdateModelPriceCurrencyByJSONString(oldCurrencies))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
		operation_setting.USDExchangeRate = oldUSD
		operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate = oldCustom
	})
	operation_setting.USDExchangeRate = 7
	operation_setting.GetGeneralSetting().CustomCurrencyExchangeRate = 5
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	expression := `tier("base", p * 7)`
	models := []string{"sync-local-cny", "sync-local-custom", "gemini-2.5-pro-thinking-128", "sync-local-usd"}
	modes, expressions := make(map[string]string), make(map[string]string)
	for _, name := range models {
		modes[name], expressions[name] = "tiered_expr", expression
	}
	encodedModes, err := common.Marshal(modes)
	require.NoError(t, err)
	encodedExpressions, err := common.Marshal(expressions)
	require.NoError(t, err)
	config.UpdateConfigFromMap(config.GlobalConfig.Get("billing_setting"), map[string]string{"billing_mode": string(encodedModes), "billing_expr": string(encodedExpressions)})
	require.NoError(t, ratio_setting.UpdateModelPriceCurrencyByJSONString(`{"sync-local-cny":"CNY","sync-local-custom":"CUSTOM","gemini-2.5-pro-thinking-*":"CNY","gpt-image-2":"CNY"}`))
	builtin, ok := billing_setting.GetBuiltinBillingExpr("gpt-image-2")
	require.True(t, ok)
	expressions["gpt-image-2"], modes["gpt-image-2"] = builtin, "tiered_expr"
	var items []map[string]any
	for name, expr := range expressions {
		items = append(items, map[string]any{"model_name": name, "billing_mode": "tiered_expr", "billing_expr": expr, "price_currency": "USD"})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data any = map[string]any{"billing_mode": modes, "billing_expr": expressions}
		if r.URL.Path == "/api/pricing" {
			data = items
		}
		encoded, err := common.Marshal(map[string]any{"success": true, "data": data})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(encoded)
	}))
	t.Cleanup(server.Close)
	for _, endpoint := range []string{"/api/ratio_config", "/api/pricing"} {
		t.Run(endpoint, func(t *testing.T) {
			var response struct {
				Success bool
				Data    struct {
					Differences map[string]map[string]dto.DifferenceItem
					Prices      map[string]struct {
						Current   map[string]any
						Upstreams map[string]map[string]any
					}
				}
			}
			recorder := modelManagementRequest(t, FetchUpstreamRatios, http.MethodPost, "/api/ratio_sync/fetch", map[string]any{
				"upstreams": []map[string]any{{"name": "USD", "base_url": server.URL, "endpoint": endpoint}},
			}, &response)
			require.True(t, response.Success, recorder.Body.String())
			for _, name := range models[:3] {
				assert.Equal(t, expression, response.Data.Differences[name]["billing_expr"].Current)
				assert.Equal(t, expression, response.Data.Differences[name]["billing_expr"].Upstreams["USD"], "equal source text in another currency is a real price difference")
				want := map[string]any{"billing_mode": "tiered_expr", "billing_expr": expression}
				assert.Equal(t, want, response.Data.Prices[name].Current)
				assert.Equal(t, want, response.Data.Prices[name].Upstreams["USD"], "comparison metadata must not enter the USD import draft")
			}
			for _, name := range []string{"sync-local-usd", "gpt-image-2"} {
				assert.NotContains(t, response.Data.Differences, name, "built-in expressions are USD even when the legacy model currency is marked")
				assert.NotContains(t, response.Data.Prices, name)
			}
		})
	}
}
