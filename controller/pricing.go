package controller

import (
	"maps"

	"github.com/QuantumNous/new-api/common"
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
		if common.StringsContains(item.EnableGroup, "all") {
			filtered = append(filtered, item)
			continue
		}
		for _, group := range item.EnableGroup {
			if _, ok := usableGroup[group]; ok {
				filtered = append(filtered, item)
				break
			}
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
	pricing := model.GetPricing()
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	groupRatio := map[string]float64{}
	maps.Copy(groupRatio, ratio_setting.GetGroupRatioCopy())
	var group string
	if exists {
		user, err := model.GetUserCache(userId.(int))
		if err == nil {
			group = user.Group
			for g := range groupRatio {
				ratio, ok := ratio_setting.GetGroupGroupRatio(group, g)
				if ok {
					groupRatio[g] = ratio
				}
			}
		}
	}

	usableGroup = service.GetUserUsableGroups(group)
	pricing = filterPricingByUsableGroups(pricing, usableGroup)
	// check groupRatio contains usableGroup
	for group := range ratio_setting.GetGroupRatioCopy() {
		if _, ok := usableGroup[group]; !ok {
			delete(groupRatio, group)
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
		rule := setting.ResolveModelRequestRateLimit(group, g, g)
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
		"auto_groups":           service.GetUserAutoGroup(group),
		"config_groups":         service.GetUserConfigGroups(group),
		"group_rate_limits":     groupRateLimits,
		"group_balance_buckets": groupBalanceBuckets,
		"pricing_version":       "a42d372ccf0b5dd13ecf71203521f9d2",
	})
}

func ResetModelRatio(c *gin.Context) {
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "重置模型倍率成功",
	})
}
