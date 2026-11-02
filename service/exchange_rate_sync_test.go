package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncUSDExchangeRate(t *testing.T) {
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))

	oversized := `{"result":"success","rates":{"CNY":7.2},"pad":"` + strings.Repeat("x", exchangeRateMaxResponseBytes) + `"}`
	cases := []struct {
		name      string
		status    int
		body      string
		wantRate  float64
		wantError string
	}{
		{name: "valid response updates the rate", status: http.StatusOK, body: `{"result":"success","base_code":"USD","rates":{"USD":1,"CNY":7.1234}}`, wantRate: 7.1234},
		{name: "rate below range is rejected", status: http.StatusOK, body: `{"result":"success","rates":{"CNY":0.5}}`, wantRate: 7.3, wantError: exchangeRateOutOfRangeKey},
		{name: "rate above range is rejected", status: http.StatusOK, body: `{"result":"success","rates":{"CNY":150}}`, wantRate: 7.3, wantError: exchangeRateOutOfRangeKey},
		{name: "zero rate is rejected", status: http.StatusOK, body: `{"result":"success","rates":{"CNY":0}}`, wantRate: 7.3, wantError: exchangeRateOutOfRangeKey},
		{name: "missing CNY keeps old rate", status: http.StatusOK, body: `{"result":"success","rates":{"EUR":0.9}}`, wantRate: 7.3, wantError: exchangeRateInvalidResponseKey},
		{name: "error result keeps old rate", status: http.StatusOK, body: `{"result":"error","error-type":"unsupported-code"}`, wantRate: 7.3, wantError: exchangeRateInvalidResponseKey},
		{name: "malformed JSON keeps old rate", status: http.StatusOK, body: `{"result":`, wantRate: 7.3, wantError: exchangeRateInvalidResponseKey},
		{name: "oversized body keeps old rate", status: http.StatusOK, body: oversized, wantRate: 7.3, wantError: exchangeRateInvalidResponseKey},
		{name: "upstream error status keeps old rate", status: http.StatusServiceUnavailable, body: `{}`, wantRate: 7.3, wantError: exchangeRateRequestFailedKey},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, model.DB.Where("1 = 1").Delete(&model.Option{}).Error)
			oldRate := operation_setting.USDExchangeRate
			oldSetting := *operation_setting.GetExchangeRateSetting()
			common.OptionMapRWMutex.Lock()
			oldOptionMap := common.OptionMap
			common.OptionMap = map[string]string{}
			common.OptionMapRWMutex.Unlock()
			t.Cleanup(func() {
				operation_setting.USDExchangeRate = oldRate
				*operation_setting.GetExchangeRateSetting() = oldSetting
				common.OptionMapRWMutex.Lock()
				common.OptionMap = oldOptionMap
				common.OptionMapRWMutex.Unlock()
			})
			operation_setting.USDExchangeRate = 7.3

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			source := &OpenERAPISource{URL: server.URL, Client: server.Client()}

			result, err := SyncUSDExchangeRate(context.Background(), source)

			require.NotNil(t, result)
			if tc.wantError == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Equal(t, tc.wantError, err.Error())
			}
			assert.Equal(t, tc.wantRate, result.USDExchangeRate)
			assert.Equal(t, tc.wantError, result.LastError)
			assert.Equal(t, exchangeRateOpenERAPISource, result.LastSource)
			assert.Positive(t, result.LastSyncedAt)

			assert.Equal(t, tc.wantRate, operation_setting.USDExchangeRate)
			status := operation_setting.GetExchangeRateSetting()
			assert.Equal(t, tc.wantError, status.LastError)
			assert.Equal(t, result.LastSyncedAt, status.LastSyncedAt)

			var stored []model.Option
			require.NoError(t, model.DB.Find(&stored).Error)
			persisted := map[string]string{}
			for _, option := range stored {
				persisted[option.Key] = option.Value
			}
			assert.Equal(t, tc.wantError, persisted[exchangeRateSyncOptionKeyErr])
			if tc.wantError == "" {
				assert.Equal(t, "7.1234", persisted[exchangeRateUSDOptionKey])
			} else {
				assert.NotContains(t, persisted, exchangeRateUSDOptionKey)
			}
		})
	}
}
