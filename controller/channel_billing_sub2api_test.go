package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupSub2APIBalanceTest wires an in-memory SQLite DB into model.DB so
// Channel.UpdateBalance can persist, mirroring the pattern used by other
// controller tests.
func setupSub2APIBalanceTest(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Discard,
	})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.Channel{}))
	model.DB = database
	t.Cleanup(func() {
		model.DB = originalDB
	})
}

func newSub2APITestChannel(t *testing.T, serverURL string) *model.Channel {
	t.Helper()
	channel := &model.Channel{
		Type:    constant.ChannelTypeSub2API,
		BaseURL: &serverURL,
		Key:     "sk-test",
		Name:    "sub2api-test",
	}
	require.NoError(t, model.DB.Create(channel).Error)
	return channel
}

func TestUpdateChannelSub2APIBalanceRemainingPreferred(t *testing.T) {
	setupSub2APIBalanceTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/usage", r.URL.Path)
		assert.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"balance": 100.0, "remaining": 986.17194888, "isValid": true, "unit": "USD"}`))
	}))
	defer server.Close()

	channel := newSub2APITestChannel(t, server.URL)
	balance, err := updateChannelSub2APIBalance(channel)
	require.NoError(t, err)
	assert.InDelta(t, 986.17194888, balance, 1e-9)
	assert.InDelta(t, 986.17194888, channel.Balance, 1e-9)
}

func TestUpdateChannelSub2APIBalanceWalletFallback(t *testing.T) {
	setupSub2APIBalanceTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"balance": 42.5, "isValid": true, "unit": "USD"}`))
	}))
	defer server.Close()

	channel := newSub2APITestChannel(t, server.URL)
	balance, err := updateChannelSub2APIBalance(channel)
	require.NoError(t, err)
	assert.InDelta(t, 42.5, balance, 1e-9)
}

func TestUpdateChannelSub2APIBalanceSubscriptionKeyMissingFields(t *testing.T) {
	setupSub2APIBalanceTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"balance": null, "remaining": null, "isValid": true, "mode": "unrestricted"}`))
	}))
	defer server.Close()

	channel := newSub2APITestChannel(t, server.URL)
	_, err := updateChannelSub2APIBalance(channel)
	require.Error(t, err)
}

func TestUpdateChannelSub2APIBalanceInvalidKey(t *testing.T) {
	setupSub2APIBalanceTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"isValid": false}`))
	}))
	defer server.Close()

	channel := newSub2APITestChannel(t, server.URL)
	_, err := updateChannelSub2APIBalance(channel)
	require.EqualError(t, err, "Sub2API key is invalid (isValid=false)")
}

func TestUpdateChannelSub2APIBalanceUnlimitedSubscription(t *testing.T) {
	setupSub2APIBalanceTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"remaining": -1, "isValid": true, "mode": "unrestricted", "subscription": {"daily_limit_usd": null, "weekly_limit_usd": null, "monthly_limit_usd": null}}`))
	}))
	defer server.Close()

	channel := newSub2APITestChannel(t, server.URL)
	require.NoError(t, model.DB.Model(channel).Update("balance", 12.5).Error)

	_, err := updateChannelSub2APIBalance(channel)
	require.EqualError(t, err, "Sub2API /v1/usage reports an unlimited subscription without a finite balance")

	stored := model.Channel{}
	require.NoError(t, model.DB.First(&stored, channel.Id).Error)
	assert.InDelta(t, 12.5, stored.Balance, 1e-9)
}

func TestUpdateChannelSub2APIBalanceNegativeWalletBalance(t *testing.T) {
	setupSub2APIBalanceTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"balance": -1, "remaining": -1, "isValid": true, "mode": "unrestricted", "unit": "USD"}`))
	}))
	defer server.Close()

	channel := newSub2APITestChannel(t, server.URL)
	balance, err := updateChannelSub2APIBalance(channel)
	require.NoError(t, err)
	assert.InDelta(t, -1, balance, 1e-9)
}

func TestUpdateChannelSub2APIBalanceMissingBaseURL(t *testing.T) {
	setupSub2APIBalanceTest(t)
	empty := ""
	channel := &model.Channel{BaseURL: &empty, Key: "sk-test"}
	_, err := updateChannelSub2APIBalance(channel)
	require.Error(t, err)
}

func TestUpdateStandardChannelBalanceRoutesSub2API(t *testing.T) {
	setupSub2APIBalanceTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"balance": 77.7, "isValid": true, "unit": "USD"}`))
	}))
	defer server.Close()

	channel := newSub2APITestChannel(t, server.URL)
	result, err := updateChannelBalance(channel)
	require.NoError(t, err)
	assert.InDelta(t, 77.7, result.Balance, 1e-9)
}

func TestSub2APIRatioProbeAndSync(t *testing.T) {
	setupSub2APIBalanceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelCostRatioHistory{}, &model.ChannelCostRatioSyncStatus{}))
	valid := `{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":0.4,"user_rate_multiplier":0.2,"resolved_rate_multiplier":0.2,"peak_rate_enabled":true,"effective_rate_multiplier":0.8,"observed_at":"2026-11-03T10:00:00Z"}`
	body := valid
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/sub2api/billing", r.URL.Path)
		assert.Equal(t, "Bearer first-key", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	channel := newSub2APITestChannel(t, server.URL)
	channel.Key = "first-key\nsecond-key"
	channel.ChannelInfo.IsMultiKey = true
	channel.SetSetting(dto.ChannelSettings{UpstreamCostRatio: common.GetPointer(.35), UpstreamCostRatioSyncMin: common.GetPointer(.2), UpstreamCostRatioSyncMax: common.GetPointer(.5)})
	require.NoError(t, model.DB.Save(channel).Error)
	probe, err := probeSub2APIRatio(channel)
	require.NoError(t, err)
	assert.True(t, probe.InRange)
	assert.True(t, probe.PeakRateEnabled)
	assert.Equal(t, .2, *probe.ResolvedRateMultiplier, "peak multiplier must not be applied")
	var count int64
	require.NoError(t, model.DB.Model(&model.ChannelCostRatioHistory{}).Count(&count).Error)
	assert.Zero(t, count, "probe is read only")
	channel.Key = `["first-key","second-key"]`
	_, err = probeSub2APIRatio(channel)
	require.NoError(t, err)
	result, err := syncSub2APIRatio(channel, false)
	require.NoError(t, err)
	assert.Equal(t, "success", result.Status)
	assert.Equal(t, .35, *result.OldRatio)
	assert.Equal(t, .2, *result.NewRatio)
	items, _, status, err := model.GetChannelCostRatioHistory(channel.Id, 1, 20)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.JSONEq(t, `{"group_rate_multiplier":0.4,"user_rate_multiplier":0.2,"resolved_rate_multiplier":0.2,"observed_at":"2026-11-03T10:00:00Z"}`, items[0].UpstreamInfo)
	require.NotNil(t, status)
	firstSuccess := status.LastSuccessAt
	result, err = syncSub2APIRatio(channel, false)
	require.NoError(t, err)
	assert.Equal(t, "unchanged", result.Status)
	body = strings.Replace(valid, `"resolved_rate_multiplier":0.2`, `"resolved_rate_multiplier":0.6`, 1)
	result, err = syncSub2APIRatio(channel, false)
	require.NoError(t, err)
	assert.Equal(t, "out_of_range", result.Status)
	assert.Equal(t, .2, *result.NewRatio)
	assert.Equal(t, .6, *result.Value)

	for _, tc := range []struct{ name, from, to, want string }{
		{"object", "sub2api.key_billing", "other.billing", "unsupported"},
		{"schema", `"schema_version":1`, `"schema_version":2`, "unsupported"},
		{"schema type", `"schema_version":1`, `"schema_version":"1"`, "unsupported"},
		{"scope", `"billing_scope":"token"`, `"billing_scope":"subscription"`, "unsupported"},
		{"missing value", `"resolved_rate_multiplier":0.2`, `"resolved_rate_multiplier":null`, "failed"},
		{"negative value", `"resolved_rate_multiplier":0.2`, `"resolved_rate_multiplier":-1`, "failed"},
		{"excessive value", `"resolved_rate_multiplier":0.2`, `"resolved_rate_multiplier":101`, "failed"},
		{"string value", `"resolved_rate_multiplier":0.2`, `"resolved_rate_multiplier":"0.2"`, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body = strings.Replace(valid, tc.from, tc.to, 1)
			result, err := syncSub2APIRatio(channel, false)
			require.NoError(t, err)
			assert.Equal(t, tc.want, result.Status)
			assert.NotEmpty(t, result.Error)
		})
	}
	body = strings.Repeat("x", 64<<10+1)
	_, err = probeSub2APIRatio(channel)
	require.ErrorContains(t, err, "response exceeds 65536 bytes")
	items, total, status, err := model.GetChannelCostRatioHistory(channel.Id, 1, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	assert.Len(t, items, 1)
	assert.Equal(t, firstSuccess, status.LastSuccessAt)
	assert.Equal(t, "failed", status.Status)
	channel.Type = constant.ChannelTypeOpenAI
	_, err = probeSub2APIRatio(channel)
	require.EqualError(t, err, model.OnlySub2APIRatioSync)
}

func TestSub2APIRatioEndpoints(t *testing.T) {
	setupSub2APIBalanceTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.ChannelCostRatioHistory{}, &model.ChannelCostRatioSyncStatus{}))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"object":"sub2api.key_billing","schema_version":1,"billing_scope":"token","group_rate_multiplier":0.9,"resolved_rate_multiplier":0.9,"observed_at":"2026-11-03T10:00:00Z"}`))
	}))
	defer server.Close()
	channel := newSub2APITestChannel(t, server.URL)
	channel.SetSetting(dto.ChannelSettings{UpstreamCostRatio: common.GetPointer(.3), UpstreamCostRatioSyncMax: common.GetPointer(.5)})
	require.NoError(t, model.DB.Save(channel).Error)
	engine := gin.New()
	engine.GET("/:id/probe", ProbeChannelUpstreamCostRatio)
	engine.POST("/:id/sync", SyncChannelUpstreamCostRatio)
	engine.GET("/:id/history", GetChannelUpstreamCostRatioHistory)
	prefix := "/" + strconv.Itoa(channel.Id)
	for _, tc := range []struct {
		method, path string
		check        func(*httptest.ResponseRecorder)
	}{
		{http.MethodGet, "/history", func(w *httptest.ResponseRecorder) {
			assert.JSONEq(t, `{"success":true,"message":"","data":{"items":[],"total":0,"page":1,"page_size":20,"sync_status":null}}`, w.Body.String())
		}},
		{http.MethodGet, "/probe", func(w *httptest.ResponseRecorder) {
			assert.False(t, gjson.Get(w.Body.String(), "data.in_range").Bool())
			assert.Equal(t, gjson.Null, gjson.Get(w.Body.String(), "data.user_rate_multiplier").Type)
		}},
		{http.MethodPost, "/sync", func(w *httptest.ResponseRecorder) {
			assert.JSONEq(t, `{"success":true,"message":"","data":{"status":"out_of_range","old_ratio":0.3,"new_ratio":0.3,"value":0.9,"error":""}}`, w.Body.String())
		}},
		{http.MethodGet, "/history?p=1&page_size=20", func(w *httptest.ResponseRecorder) {
			assert.Equal(t, "out_of_range", gjson.Get(w.Body.String(), "data.sync_status.status").String())
			assert.Zero(t, gjson.Get(w.Body.String(), "data.total").Int())
		}},
	} {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(tc.method, prefix+tc.path, nil))
		assert.Equal(t, http.StatusOK, w.Code)
		tc.check(w)
	}
}
