package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pricingGroupInfoResponse struct {
	Success             bool                             `json:"success"`
	ConfigGroups        []service.UserConfigGroup        `json:"config_groups"`
	GroupRateLimits     map[string]pricingGroupRateLimit `json:"group_rate_limits"`
	GroupBalanceBuckets map[string][]string              `json:"group_balance_buckets"`
}

// withPricingGroupSettings 配置可用分组、配置分组、余额类型与限流，并在测试结束后还原。
func withPricingGroupSettings(t *testing.T, rateLimitEnabled bool) {
	t.Helper()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalRatios := ratio_setting.GroupRatio2JSONString()
	originalConfigGroups := setting.ConfigGroups2JSONString()
	originalBuckets := setting.GroupBalanceBuckets2JSONString()
	originalPrivate := setting.ModelRequestRateLimitPrivateGroup2JSONString()
	originalGlobal := setting.ModelRequestRateLimitGlobalGroup2JSONString()
	originalLegacy := setting.ModelRequestRateLimitGroup2JSONString()
	originalEnabled := setting.ModelRequestRateLimitEnabled
	originalDuration := setting.ModelRequestRateLimitDurationMinutes
	originalCount := setting.ModelRequestRateLimitCount
	originalSuccess := setting.ModelRequestRateLimitSuccessCount
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatios))
		require.NoError(t, setting.UpdateConfigGroupsByJSONString(originalConfigGroups))
		require.NoError(t, setting.UpdateGroupBalanceBucketsByJSONString(originalBuckets))
		require.NoError(t, setting.UpdateModelRequestRateLimitPrivateGroupByJSONString(originalPrivate))
		require.NoError(t, setting.UpdateModelRequestRateLimitGlobalGroupByJSONString(originalGlobal))
		require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(originalLegacy))
		setting.ModelRequestRateLimitEnabled = originalEnabled
		setting.ModelRequestRateLimitDurationMinutes = originalDuration
		setting.ModelRequestRateLimitCount = originalCount
		setting.ModelRequestRateLimitSuccessCount = originalSuccess
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP","svip":"SVIP","auto":"Auto"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2,"svip":3}`))
	require.NoError(t, setting.UpdateConfigGroupsByJSONString(`[
		{"key":"public","name":"Public","description":"Open plan","groups":["svip","vip"]},
		{"key":"vip-only","groups":["vip","default"],"user_groups":["vip"]},
		{"key":"ghost","groups":["ghost"]}
	]`))
	require.NoError(t, setting.UpdateGroupBalanceBucketsByJSONString(`{"vip":["topup"],"*":["gift","topup"]}`))
	require.NoError(t, setting.UpdateModelRequestRateLimitPrivateGroupByJSONString(`{"vip":{"svip":{"total":5,"success":3}},"default":{"*":{"total":7,"success":7,"duration":2}}}`))
	require.NoError(t, setting.UpdateModelRequestRateLimitGlobalGroupByJSONString(`{"svip":{"total":100,"success":50,"duration":5}}`))
	require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(`{"vip":[40,20]}`))
	setting.ModelRequestRateLimitEnabled = rateLimitEnabled
	setting.ModelRequestRateLimitDurationMinutes = 1
	setting.ModelRequestRateLimitCount = 0
	setting.ModelRequestRateLimitSuccessCount = 1000
}

func TestGetPricingExposesGroupInfoForCurrentUser(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{Id: 3101, Username: "pricing_default", AffCode: "pricing_default", Group: "default", Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.User{Id: 3102, Username: "pricing_vip", AffCode: "pricing_vip", Group: "vip", Status: common.UserStatusEnabled}).Error)

	defaultLimit := pricingGroupRateLimit{Total: 0, Success: 1000, DurationMinutes: 1}
	svipGlobal := pricingGroupRateLimit{Total: 100, Success: 50, DurationMinutes: 5}
	vipLegacy := pricingGroupRateLimit{Total: 40, Success: 20, DurationMinutes: 1}
	defaultWildcard := pricingGroupRateLimit{Total: 7, Success: 7, DurationMinutes: 2}
	wantBuckets := map[string][]string{
		"default": {"gift", "topup"},
		"vip":     {"topup"},
		"svip":    {"gift", "topup"},
	}

	tests := []struct {
		name             string
		userID           int
		rateLimitEnabled bool
		wantRateLimits   map[string]pricingGroupRateLimit
		wantConfigGroups []string
	}{
		{
			// 匿名访客只看到全局 / 旧版 / 默认规则，任何用户分组的私有规则都不出现。
			name:             "anonymous sees public rules only",
			rateLimitEnabled: true,
			wantRateLimits:   map[string]pricingGroupRateLimit{"default": defaultLimit, "vip": vipLegacy, "svip": svipGlobal},
			wantConfigGroups: []string{"public"},
		},
		{
			// default 用户命中自己的私有通配规则，看不到 vip 的私有规则。
			name:             "default user sees own private wildcard",
			userID:           3101,
			rateLimitEnabled: true,
			wantRateLimits:   map[string]pricingGroupRateLimit{"default": defaultWildcard, "vip": defaultWildcard, "svip": defaultWildcard},
			wantConfigGroups: []string{"public"},
		},
		{
			name:             "vip user sees own private rule and restricted config group",
			userID:           3102,
			rateLimitEnabled: true,
			wantRateLimits: map[string]pricingGroupRateLimit{
				"default": defaultLimit,
				"vip":     vipLegacy,
				"svip":    {Total: 5, Success: 3, DurationMinutes: 1},
			},
			wantConfigGroups: []string{"public", "vip-only"},
		},
		{
			name:             "rate limiting disabled returns no limits",
			userID:           3102,
			wantRateLimits:   map[string]pricingGroupRateLimit{},
			wantConfigGroups: []string{"public", "vip-only"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withPricingGroupSettings(t, tt.rateLimitEnabled)

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/api/pricing", nil)
			if tt.userID != 0 {
				ctx.Set("id", tt.userID)
			}
			GetPricing(ctx)

			require.Equal(t, http.StatusOK, recorder.Code)
			var resp pricingGroupInfoResponse
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
			require.True(t, resp.Success)

			assert.Equal(t, tt.wantRateLimits, resp.GroupRateLimits)
			assert.Equal(t, wantBuckets, resp.GroupBalanceBuckets)

			keys := make([]string, 0, len(resp.ConfigGroups))
			for _, cfg := range resp.ConfigGroups {
				keys = append(keys, cfg.Key)
			}
			assert.Equal(t, tt.wantConfigGroups, keys)
			assert.Equal(t, service.UserConfigGroup{
				Key: "public", Ref: setting.ConfigGroupRef("public"), Name: "Public",
				Description: "Open plan", Groups: []string{"svip", "vip"},
			}, resp.ConfigGroups[0])
		})
	}
}
