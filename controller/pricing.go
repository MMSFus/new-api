package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func filterPricingByUsableGroups(pricing []model.Pricing, usableGroup map[string]string) []model.Pricing {
	if len(pricing) == 0 {
		return pricing
	}
	if len(usableGroup) == 0 {
		return []model.Pricing{}
	}

	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		// The pricing cache is shared by all viewers.
		groups := make([]string, 0, len(item.EnableGroup))
		for _, group := range item.EnableGroup {
			if _, ok := usableGroup[group]; ok || group == "all" {
				groups = append(groups, group)
			}
		}
		if len(groups) > 0 {
			item.EnableGroup = groups
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// pricingGroupRateLimit 是当前用户调用某分组时生效的模型请求限流。
// Total / Success 为 0 表示不限制。
type pricingGroupRateLimit struct {
	Total           int `json:"total"`
	Success         int `json:"success"`
	DurationMinutes int `json:"duration"`
}

func GetPricing(c *gin.Context) {
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	if userGroup == "" && c.GetInt("id") > 0 {
		if user, err := model.GetUserCache(c.GetInt("id")); err == nil {
			userGroup = user.Group
		}
	}
	usableGroup := service.GetUserUsableGroups(userGroup)
	pricing := filterPricingByUsableGroups(model.GetPricing(), usableGroup)
	groupRatio := ratio_setting.GetGroupRatioCopy()
	for group := range groupRatio {
		if _, ok := usableGroup[group]; !ok {
			delete(groupRatio, group)
			continue
		}
		if userGroup != "" {
			if ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, group); ok {
				groupRatio[group] = ratio
			}
		}
	}

	// 按当前用户分组解析每个可用分组的限流与可用余额类型。匿名访客的
	// group 为空，ResolveModelRequestRateLimit 会跳过私有规则，只返回对
	// 所有人生效的全局 / 旧版 / 默认规则，不会泄露其他用户分组的私有规则。
	groupRateLimits := map[string]pricingGroupRateLimit{}
	groupBalanceBuckets := make(map[string][]string, len(usableGroup))
	for g := range usableGroup {
		if g == "auto" {
			continue
		}
		groupBalanceBuckets[g] = setting.GetGroupBalanceBuckets(g)
		if !setting.ModelRequestRateLimitEnabled {
			continue
		}
		// 固定为 g 的令牌：旧版限流表按令牌分组查找。
		rule := setting.ResolveModelRequestRateLimit(userGroup, g, g)
		groupRateLimits[g] = pricingGroupRateLimit{
			Total:           rule.Total,
			Success:         rule.Success,
			DurationMinutes: rule.DurationMinutes,
		}
	}

	c.JSON(200, gin.H{
		"success":               true,
		"data":                  pricing,
		"vendors":               model.GetVendors(),
		"group_ratio":           groupRatio,
		"usable_group":          usableGroup,
		"supported_endpoint":    model.GetSupportedEndpointMap(),
		"auto_groups":           service.GetUserAutoGroup(userGroup),
		"config_groups":         service.GetUserConfigGroups(userGroup),
		"group_rate_limits":     groupRateLimits,
		"group_balance_buckets": groupBalanceBuckets,
		"pricing_version":       "a42d372ccf0b5dd13ecf71203521f9d2",
	})
}

func ResetModelRatio(c *gin.Context) {
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccessT(c, "Model ratios reset", nil)
}
