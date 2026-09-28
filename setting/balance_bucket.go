package setting

import (
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

// GroupBalanceBucketsOptionKey 配置每个分组可用的余额桶及扣费顺序，
// JSON 格式：{"vip": ["topup"], "default": ["gift", "topup"]}。
// 键 "*" 可覆盖未单独配置分组的默认顺序；未配置时所有分组可用全部余额。
const GroupBalanceBucketsOptionKey = "GroupBalanceBuckets"

// GroupBalanceBucketsFallbackKey 为未单独配置的分组提供默认规则。
const GroupBalanceBucketsFallbackKey = "*"

var (
	groupBalanceBuckets      = map[string][]string{}
	groupBalanceBucketsMutex sync.RWMutex
)

// ParseGroupBalanceBuckets 解析并校验分组余额配置。
func ParseGroupBalanceBuckets(jsonStr string) (map[string][]string, error) {
	parsed := map[string][]string{}
	if strings.TrimSpace(jsonStr) == "" {
		return parsed, nil
	}
	if err := common.UnmarshalJsonStr(jsonStr, &parsed); err != nil {
		return nil, fmt.Errorf("分组可用余额配置不是合法的 JSON: %w", err)
	}
	normalized := make(map[string][]string, len(parsed))
	for group, buckets := range parsed {
		name := strings.TrimSpace(group)
		if name == "" {
			return nil, fmt.Errorf("分组名不能为空")
		}
		if len(buckets) == 0 {
			return nil, fmt.Errorf("分组 %s 至少需要一种可用余额", name)
		}
		seen := make(map[string]bool, len(buckets))
		list := make([]string, 0, len(buckets))
		for _, bucket := range buckets {
			bucket = strings.TrimSpace(bucket)
			if !common.IsBalanceBucket(bucket) {
				return nil, fmt.Errorf("分组 %s 包含未知余额类型 %q", name, bucket)
			}
			if seen[bucket] {
				return nil, fmt.Errorf("分组 %s 的余额类型 %s 重复", name, bucket)
			}
			seen[bucket] = true
			list = append(list, bucket)
		}
		if _, dup := normalized[name]; dup {
			return nil, fmt.Errorf("分组 %s 重复配置", name)
		}
		normalized[name] = list
	}
	return normalized, nil
}

// ValidateGroupBalanceBuckets 校验配置字符串。
func ValidateGroupBalanceBuckets(jsonStr string) error {
	_, err := ParseGroupBalanceBuckets(jsonStr)
	return err
}

// UpdateGroupBalanceBucketsByJSONString 热更新配置。
func UpdateGroupBalanceBucketsByJSONString(jsonStr string) error {
	parsed, err := ParseGroupBalanceBuckets(jsonStr)
	if err != nil {
		return err
	}
	groupBalanceBucketsMutex.Lock()
	groupBalanceBuckets = parsed
	groupBalanceBucketsMutex.Unlock()
	return nil
}

// GroupBalanceBuckets2JSONString 序列化当前配置。
func GroupBalanceBuckets2JSONString() string {
	groupBalanceBucketsMutex.RLock()
	defer groupBalanceBucketsMutex.RUnlock()
	data, err := common.Marshal(groupBalanceBuckets)
	if err != nil {
		common.SysLog("error marshalling group balance buckets: " + err.Error())
		return "{}"
	}
	return string(data)
}

// GetGroupBalanceBuckets 返回分组可用的余额桶（按扣费顺序）。
// 分组未配置时依次回退到 "*" 规则与全部余额的默认顺序。
func GetGroupBalanceBuckets(group string) []string {
	groupBalanceBucketsMutex.RLock()
	defer groupBalanceBucketsMutex.RUnlock()
	if group != "" {
		if buckets, ok := groupBalanceBuckets[group]; ok {
			return append([]string(nil), buckets...)
		}
	}
	if buckets, ok := groupBalanceBuckets[GroupBalanceBucketsFallbackKey]; ok {
		return append([]string(nil), buckets...)
	}
	return common.DefaultBalanceBucketOrder()
}
