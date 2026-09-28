package setting

import (
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// ConfigGroupsOptionKey is the option that stores admin-defined config
// groups: named, ordered Auto group lists that users can pick for a token.
const ConfigGroupsOptionKey = "ConfigGroups"

// ConfigGroupRefPrefix marks a token group value that references a config
// group instead of a real group, e.g. "cfg:claude-best". The prefix keeps
// references unambiguous even if a real group with the same name is added
// later.
const ConfigGroupRefPrefix = "cfg:"

const (
	MaxConfigGroups           = 50
	MaxConfigGroupMembers     = 20
	MaxConfigGroupKeyLength   = 32
	MaxConfigGroupNameLength  = 64
	MaxConfigGroupDescLength  = 255
	MaxConfigGroupUserGroups  = 50
	configGroupAutoReserved   = "auto"
	configGroupDisallowedRune = ":,"
)

// ConfigGroup is one admin-defined Auto routing plan.
type ConfigGroup struct {
	// Key is the stable identifier stored in tokens (with ConfigGroupRefPrefix).
	Key string `json:"key"`
	// Name is the display name shown to users; falls back to Key.
	Name        string `json:"name"`
	Description string `json:"description"`
	// Groups is the ordered list of real groups to try, highest priority first.
	Groups []string `json:"groups"`
	// UserGroups restricts which user groups may see/use the plan. Empty
	// means every user group.
	UserGroups []string `json:"user_groups"`
	// CrossGroupRetry is the default cross-group retry switch applied when a
	// user selects this plan for a token.
	CrossGroupRetry bool `json:"cross_group_retry"`
}

// DisplayName returns Name or Key when Name is empty.
func (g ConfigGroup) DisplayName() string {
	if g.Name != "" {
		return g.Name
	}
	return g.Key
}

// AllowsUserGroup reports whether the plan is visible to userGroup.
func (g ConfigGroup) AllowsUserGroup(userGroup string) bool {
	return len(g.UserGroups) == 0 || slices.Contains(g.UserGroups, userGroup)
}

func (g ConfigGroup) clone() ConfigGroup {
	g.Groups = slices.Clone(g.Groups)
	g.UserGroups = slices.Clone(g.UserGroups)
	return g
}

type configGroupSnapshot struct {
	list  []ConfigGroup
	byKey map[string]ConfigGroup
}

var configGroups atomic.Pointer[configGroupSnapshot]

func init() {
	configGroups.Store(&configGroupSnapshot{byKey: map[string]ConfigGroup{}})
}

// ConfigGroupRef returns the token group value that references key.
func ConfigGroupRef(key string) string {
	return ConfigGroupRefPrefix + key
}

// ParseConfigGroupRef extracts the config group key from a token group value.
func ParseConfigGroupRef(group string) (string, bool) {
	key, ok := strings.CutPrefix(group, ConfigGroupRefPrefix)
	if !ok || key == "" {
		return "", false
	}
	return key, true
}

// IsConfigGroupRef reports whether group references a config group.
func IsConfigGroupRef(group string) bool {
	_, ok := ParseConfigGroupRef(group)
	return ok
}

// ParseConfigGroups decodes and structurally validates the option value.
// It does not check that referenced groups exist, so it is safe to use while
// options are still loading at startup.
func ParseConfigGroups(jsonString string) ([]ConfigGroup, error) {
	trimmed := strings.TrimSpace(jsonString)
	if trimmed == "" || trimmed == "null" {
		return []ConfigGroup{}, nil
	}
	var list []ConfigGroup
	if err := common.UnmarshalJsonStr(trimmed, &list); err != nil {
		return nil, fmt.Errorf("config groups must be a JSON array: %w", err)
	}
	if len(list) > MaxConfigGroups {
		return nil, fmt.Errorf("at most %d config groups are allowed", MaxConfigGroups)
	}
	seenKeys := make(map[string]struct{}, len(list))
	for i := range list {
		item := &list[i]
		item.Key = strings.TrimSpace(item.Key)
		item.Name = strings.TrimSpace(item.Name)
		item.Description = strings.TrimSpace(item.Description)
		if err := validateConfigGroupKey(item.Key); err != nil {
			return nil, fmt.Errorf("config group #%d: %w", i+1, err)
		}
		if _, ok := seenKeys[item.Key]; ok {
			return nil, fmt.Errorf("config group key %q is duplicated", item.Key)
		}
		seenKeys[item.Key] = struct{}{}
		if utf8.RuneCountInString(item.Name) > MaxConfigGroupNameLength {
			return nil, fmt.Errorf("config group %q: name exceeds %d characters", item.Key, MaxConfigGroupNameLength)
		}
		if utf8.RuneCountInString(item.Description) > MaxConfigGroupDescLength {
			return nil, fmt.Errorf("config group %q: description exceeds %d characters", item.Key, MaxConfigGroupDescLength)
		}
		groups, err := normalizeGroupList(item.Groups, MaxConfigGroupMembers, false)
		if err != nil {
			return nil, fmt.Errorf("config group %q groups: %w", item.Key, err)
		}
		if len(groups) == 0 {
			return nil, fmt.Errorf("config group %q must contain at least one group", item.Key)
		}
		for _, group := range groups {
			if group == configGroupAutoReserved || IsConfigGroupRef(group) {
				return nil, fmt.Errorf("config group %q cannot reference %q", item.Key, group)
			}
		}
		item.Groups = groups
		userGroups, err := normalizeGroupList(item.UserGroups, MaxConfigGroupUserGroups, true)
		if err != nil {
			return nil, fmt.Errorf("config group %q user_groups: %w", item.Key, err)
		}
		item.UserGroups = userGroups
	}
	return list, nil
}

func validateConfigGroupKey(key string) error {
	if key == "" {
		return fmt.Errorf("key is required")
	}
	if utf8.RuneCountInString(key) > MaxConfigGroupKeyLength {
		return fmt.Errorf("key %q exceeds %d characters", key, MaxConfigGroupKeyLength)
	}
	if strings.EqualFold(key, configGroupAutoReserved) {
		return fmt.Errorf("key %q is reserved", key)
	}
	for _, r := range key {
		if unicode.IsSpace(r) || unicode.IsControl(r) || strings.ContainsRune(configGroupDisallowedRune, r) {
			return fmt.Errorf("key %q must not contain whitespace, ':' or ','", key)
		}
	}
	return nil
}

func normalizeGroupList(groups []string, limit int, dedupe bool) ([]string, error) {
	out := make([]string, 0, len(groups))
	seen := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			return nil, fmt.Errorf("group name must not be empty")
		}
		if _, ok := seen[group]; ok {
			if dedupe {
				continue
			}
			return nil, fmt.Errorf("group %q is duplicated", group)
		}
		seen[group] = struct{}{}
		out = append(out, group)
	}
	if len(out) > limit {
		return nil, fmt.Errorf("at most %d entries are allowed", limit)
	}
	return out, nil
}

// ValidateConfigGroupsJSON performs full validation for an admin update:
// structure, referenced groups exist in GroupRatio, and keys do not collide
// with real groups.
func ValidateConfigGroupsJSON(jsonString string) error {
	list, err := ParseConfigGroups(jsonString)
	if err != nil {
		return err
	}
	for _, item := range list {
		if ratio_setting.ContainsGroupRatio(item.Key) {
			return fmt.Errorf("config group key %q conflicts with an existing group", item.Key)
		}
		for _, group := range item.Groups {
			if !ratio_setting.ContainsGroupRatio(group) {
				return fmt.Errorf("config group %q references unknown group %q", item.Key, group)
			}
		}
	}
	return nil
}

// UpdateConfigGroupsByJSONString replaces the in-memory config groups. The
// swap is atomic; readers always see either the old or the new snapshot.
func UpdateConfigGroupsByJSONString(jsonString string) error {
	list, err := ParseConfigGroups(jsonString)
	if err != nil {
		return err
	}
	byKey := make(map[string]ConfigGroup, len(list))
	for _, item := range list {
		byKey[item.Key] = item
	}
	configGroups.Store(&configGroupSnapshot{list: list, byKey: byKey})
	return nil
}

// ConfigGroups2JSONString serializes the current config groups.
func ConfigGroups2JSONString() string {
	list := configGroups.Load().list
	if list == nil {
		list = []ConfigGroup{}
	}
	data, err := common.Marshal(list)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// GetConfigGroups returns a deep copy of all config groups in admin order.
func GetConfigGroups() []ConfigGroup {
	list := configGroups.Load().list
	out := make([]ConfigGroup, 0, len(list))
	for _, item := range list {
		out = append(out, item.clone())
	}
	return out
}

// GetConfigGroup returns a deep copy of the config group with key.
func GetConfigGroup(key string) (ConfigGroup, bool) {
	item, ok := configGroups.Load().byKey[key]
	if !ok {
		return ConfigGroup{}, false
	}
	return item.clone(), true
}
