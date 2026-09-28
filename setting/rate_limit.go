package setting

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

// maxRateLimitDurationSeconds is the largest window the count cap is computed
// against (24h). Token-bucket capacity is count*duration; this keeps that
// product inside int64 when the window is at most a day.
const maxRateLimitDurationSeconds = 24 * 60 * 60

// maxRateLimitDurationMinutes is the largest per-rule window, in minutes.
const maxRateLimitDurationMinutes = maxRateLimitDurationSeconds / 60

// maxModelRequestRateLimitCount is math.MaxInt64 / maxRateLimitDurationSeconds.
// It is the largest count that cannot overflow int64(count)*duration for a
// window of at most 24 hours.
const maxModelRequestRateLimitCount int64 = math.MaxInt64 / maxRateLimitDurationSeconds

// ModelRateLimitAnyGroup is the called-group wildcard of a private rule: it
// matches every group the user calls that has no exact private rule.
const ModelRateLimitAnyGroup = "*"

// Rule sources, in match priority order.
const (
	ModelRateLimitSourcePrivate         = "private"
	ModelRateLimitSourcePrivateWildcard = "private_wildcard"
	ModelRateLimitSourceGlobal          = "global"
	ModelRateLimitSourceLegacy          = "legacy"
	ModelRateLimitSourceDefault         = "default"
)

var ModelRequestRateLimitEnabled = false
var ModelRequestRateLimitDurationMinutes = 1
var ModelRequestRateLimitCount = 0
var ModelRequestRateLimitSuccessCount = 1000

// ModelRequestRateLimitGroup is the legacy per-group table
// ({"group":[total,success]}). It is looked up by the token group, falling
// back to the user group, and is kept only as a low-priority fallback.
//
// Deprecated: use ModelRequestRateLimitGlobalGroup and
// ModelRequestRateLimitPrivateGroup.
var ModelRequestRateLimitGroup = map[string][2]int{}

// ModelRequestRateLimitGlobalGroup maps a called group to its limit. Every
// user calling that group is limited by it.
var ModelRequestRateLimitGlobalGroup = map[string]GroupRateLimit{}

// ModelRequestRateLimitPrivateGroup maps user group -> called group -> limit.
// The called group may be ModelRateLimitAnyGroup.
var ModelRequestRateLimitPrivateGroup = map[string]map[string]GroupRateLimit{}

// ModelRequestRateLimitMutex guards the three group tables above.
var ModelRequestRateLimitMutex sync.RWMutex

// GroupRateLimit is one rule of the global or private table.
type GroupRateLimit struct {
	// Total caps all requests (failures included); 0 means unlimited.
	Total int `json:"total"`
	// Success caps successful requests; 0 means unlimited.
	Success int `json:"success"`
	// DurationMinutes overrides the window; 0 uses
	// ModelRequestRateLimitDurationMinutes.
	DurationMinutes int `json:"duration,omitempty"`
}

// ModelRateLimitRule is the limit resolved for one request.
type ModelRateLimitRule struct {
	Total           int
	Success         int
	DurationMinutes int
	// Scope isolates counters. It is the called group for exact rules,
	// ModelRateLimitAnyGroup for private wildcard rules, and empty for the
	// legacy table and the default limit, which keep the per-user counters.
	Scope  string
	Source string
}

func ModelRequestRateLimitGroup2JSONString() string {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	jsonBytes, err := common.Marshal(ModelRequestRateLimitGroup)
	if err != nil {
		common.SysLog("error marshalling model request rate limit group: " + err.Error())
	}
	return string(jsonBytes)
}

func ModelRequestRateLimitGlobalGroup2JSONString() string {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	jsonBytes, err := common.Marshal(ModelRequestRateLimitGlobalGroup)
	if err != nil {
		common.SysLog("error marshalling model request rate limit global group: " + err.Error())
	}
	return string(jsonBytes)
}

func ModelRequestRateLimitPrivateGroup2JSONString() string {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	jsonBytes, err := common.Marshal(ModelRequestRateLimitPrivateGroup)
	if err != nil {
		common.SysLog("error marshalling model request rate limit private group: " + err.Error())
	}
	return string(jsonBytes)
}

// UpdateModelRequestRateLimitGroupByJSONString replaces the legacy table.
// It parses first and swaps under the write lock, so a bad value leaves the
// current table in place.
func UpdateModelRequestRateLimitGroupByJSONString(jsonStr string) error {
	next := make(map[string][2]int)
	if err := unmarshalRateLimitJSON(jsonStr, &next); err != nil {
		return err
	}
	ModelRequestRateLimitMutex.Lock()
	defer ModelRequestRateLimitMutex.Unlock()
	ModelRequestRateLimitGroup = next
	return nil
}

func UpdateModelRequestRateLimitGlobalGroupByJSONString(jsonStr string) error {
	next := make(map[string]GroupRateLimit)
	if err := unmarshalRateLimitJSON(jsonStr, &next); err != nil {
		return err
	}
	ModelRequestRateLimitMutex.Lock()
	defer ModelRequestRateLimitMutex.Unlock()
	ModelRequestRateLimitGlobalGroup = next
	return nil
}

func UpdateModelRequestRateLimitPrivateGroupByJSONString(jsonStr string) error {
	next := make(map[string]map[string]GroupRateLimit)
	if err := unmarshalRateLimitJSON(jsonStr, &next); err != nil {
		return err
	}
	ModelRequestRateLimitMutex.Lock()
	defer ModelRequestRateLimitMutex.Unlock()
	ModelRequestRateLimitPrivateGroup = next
	return nil
}

func unmarshalRateLimitJSON(jsonStr string, target any) error {
	if strings.TrimSpace(jsonStr) == "" {
		return nil
	}
	return common.Unmarshal([]byte(jsonStr), target)
}

// GetGroupRateLimit looks up the legacy table.
func GetGroupRateLimit(group string) (totalCount, successCount int, found bool) {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	if ModelRequestRateLimitGroup == nil {
		return 0, 0, false
	}

	limits, found := ModelRequestRateLimitGroup[group]
	if !found {
		return 0, 0, false
	}
	return limits[0], limits[1], true
}

// ResolveModelRequestRateLimit picks the limit for a user in userGroup
// calling calledGroup. legacyGroup is the key the legacy table was always
// looked up by (token group, falling back to the user group).
//
// Priority: private exact > private wildcard > global > legacy > default.
func ResolveModelRequestRateLimit(userGroup, calledGroup, legacyGroup string) ModelRateLimitRule {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	if rules, ok := ModelRequestRateLimitPrivateGroup[userGroup]; ok && userGroup != "" {
		if limit, ok := rules[calledGroup]; ok && calledGroup != "" && calledGroup != ModelRateLimitAnyGroup {
			return limit.rule(calledGroup, ModelRateLimitSourcePrivate)
		}
		if limit, ok := rules[ModelRateLimitAnyGroup]; ok {
			return limit.rule(ModelRateLimitAnyGroup, ModelRateLimitSourcePrivateWildcard)
		}
	}
	if limit, ok := ModelRequestRateLimitGlobalGroup[calledGroup]; ok && calledGroup != "" {
		return limit.rule(calledGroup, ModelRateLimitSourceGlobal)
	}
	if limits, ok := ModelRequestRateLimitGroup[legacyGroup]; ok {
		return ModelRateLimitRule{
			Total:           limits[0],
			Success:         limits[1],
			DurationMinutes: ModelRequestRateLimitDurationMinutes,
			Source:          ModelRateLimitSourceLegacy,
		}
	}
	return ModelRateLimitRule{
		Total:           ModelRequestRateLimitCount,
		Success:         ModelRequestRateLimitSuccessCount,
		DurationMinutes: ModelRequestRateLimitDurationMinutes,
		Source:          ModelRateLimitSourceDefault,
	}
}

func (l GroupRateLimit) rule(scope, source string) ModelRateLimitRule {
	duration := l.DurationMinutes
	if duration <= 0 {
		duration = ModelRequestRateLimitDurationMinutes
	}
	return ModelRateLimitRule{
		Total:           l.Total,
		Success:         l.Success,
		DurationMinutes: duration,
		Scope:           scope,
		Source:          source,
	}
}

func CheckModelRequestRateLimitGroup(jsonStr string) error {
	checkModelRequestRateLimitGroup := make(map[string][2]int)
	err := unmarshalRateLimitJSON(jsonStr, &checkModelRequestRateLimitGroup)
	if err != nil {
		return err
	}
	for group, limits := range checkModelRequestRateLimitGroup {
		if limits[0] < 0 || limits[1] < 1 {
			return fmt.Errorf("group %s has negative rate limit values: [%d, %d]", group, limits[0], limits[1])
		}
		if int64(limits[0]) > maxModelRequestRateLimitCount || int64(limits[1]) > maxModelRequestRateLimitCount {
			return fmt.Errorf("group %s [%d, %d] exceeds max rate limit %d", group, limits[0], limits[1], maxModelRequestRateLimitCount)
		}
	}

	return nil
}

func CheckModelRequestRateLimitGlobalGroup(jsonStr string) error {
	rules := make(map[string]GroupRateLimit)
	if err := unmarshalRateLimitJSON(jsonStr, &rules); err != nil {
		return err
	}
	for group, limit := range rules {
		if err := checkRateLimitGroupName(group, false); err != nil {
			return err
		}
		if err := limit.check(group); err != nil {
			return err
		}
	}
	return nil
}

func CheckModelRequestRateLimitPrivateGroup(jsonStr string) error {
	rules := make(map[string]map[string]GroupRateLimit)
	if err := unmarshalRateLimitJSON(jsonStr, &rules); err != nil {
		return err
	}
	for userGroup, called := range rules {
		if err := checkRateLimitGroupName(userGroup, false); err != nil {
			return err
		}
		for calledGroup, limit := range called {
			if err := checkRateLimitGroupName(calledGroup, true); err != nil {
				return err
			}
			if err := limit.check(userGroup + " -> " + calledGroup); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkRateLimitGroupName(group string, allowWildcard bool) error {
	if strings.TrimSpace(group) == "" || group != strings.TrimSpace(group) {
		return fmt.Errorf("invalid group name %q", group)
	}
	if group == ModelRateLimitAnyGroup && !allowWildcard {
		return fmt.Errorf("group wildcard %q is only allowed as the called group of a private rule", group)
	}
	return nil
}

func (l GroupRateLimit) check(label string) error {
	if l.Total < 0 || l.Success < 0 {
		return fmt.Errorf("group %s has negative rate limit values: [%d, %d]", label, l.Total, l.Success)
	}
	if int64(l.Total) > maxModelRequestRateLimitCount || int64(l.Success) > maxModelRequestRateLimitCount {
		return fmt.Errorf("group %s [%d, %d] exceeds max rate limit %d", label, l.Total, l.Success, maxModelRequestRateLimitCount)
	}
	if l.DurationMinutes < 0 || l.DurationMinutes > maxRateLimitDurationMinutes {
		return fmt.Errorf("group %s duration %d must be between 0 and %d minutes", label, l.DurationMinutes, maxRateLimitDurationMinutes)
	}
	return nil
}
