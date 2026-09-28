package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

var (
	// ErrConfigGroupNotFound means the referenced config group was deleted.
	ErrConfigGroupNotFound = errors.New("config group not found")
	// ErrConfigGroupForbidden means the user group may not use the plan.
	ErrConfigGroupForbidden = errors.New("config group is not available to this user group")
	// ErrConfigGroupNoUsableGroups means every member group was filtered out.
	ErrConfigGroupNoUsableGroups = errors.New("config group has no usable groups")
)

// UserConfigGroup is a config group resolved for a particular user group:
// Groups only contains the members the user may currently use, in order.
type UserConfigGroup struct {
	Key             string   `json:"key"`
	Ref             string   `json:"value"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Groups          []string `json:"groups"`
	CrossGroupRetry bool     `json:"cross_group_retry"`
}

// FilterUserSelectableGroups keeps groups the user may use, preserving order
// and dropping duplicates. limit <= 0 means no limit.
func FilterUserSelectableGroups(userGroup string, groups []string, limit int) []string {
	capacity := len(groups)
	if limit > 0 {
		capacity = min(capacity, limit)
	}
	filtered := make([]string, 0, capacity)
	seen := make(map[string]struct{}, capacity)
	for _, group := range groups {
		if !IsUserSelectableGroup(userGroup, group) {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		filtered = append(filtered, group)
		if limit > 0 && len(filtered) == limit {
			break
		}
	}
	return filtered
}

func resolveConfigGroupForUser(userGroup string, cfg setting.ConfigGroup) (UserConfigGroup, error) {
	if !cfg.AllowsUserGroup(userGroup) {
		return UserConfigGroup{}, ErrConfigGroupForbidden
	}
	groups := FilterUserSelectableGroups(userGroup, cfg.Groups, 0)
	if len(groups) == 0 {
		return UserConfigGroup{}, ErrConfigGroupNoUsableGroups
	}
	return UserConfigGroup{
		Key:             cfg.Key,
		Ref:             setting.ConfigGroupRef(cfg.Key),
		Name:            cfg.DisplayName(),
		Description:     cfg.Description,
		Groups:          groups,
		CrossGroupRetry: cfg.CrossGroupRetry,
	}, nil
}

// ResolveUserConfigGroup resolves key for userGroup using the live settings.
func ResolveUserConfigGroup(userGroup, key string) (UserConfigGroup, error) {
	cfg, ok := setting.GetConfigGroup(key)
	if !ok {
		return UserConfigGroup{}, ErrConfigGroupNotFound
	}
	return resolveConfigGroupForUser(userGroup, cfg)
}

// GetUserConfigGroups lists the config groups userGroup can select, in admin
// order. Plans that are hidden from the user or have no usable member group
// are omitted.
func GetUserConfigGroups(userGroup string) []UserConfigGroup {
	all := setting.GetConfigGroups()
	out := make([]UserConfigGroup, 0, len(all))
	for _, cfg := range all {
		resolved, err := resolveConfigGroupForUser(userGroup, cfg)
		if err != nil {
			continue
		}
		out = append(out, resolved)
	}
	return out
}

// ApplyConfigGroupContext switches the request onto the Auto routing path
// with the config group's ordered groups. Billing and logs then use the real
// group that serves the request, exactly as for "auto".
func ApplyConfigGroupContext(c *gin.Context, resolved UserConfigGroup) {
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "auto")
	common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")
	common.SetContextKey(c, constant.ContextKeyTokenConfigGroup, resolved.Key)
	common.SetContextKey(c, constant.ContextKeyTokenConfigGroupGroups, resolved.Groups)
}

// getRequestConfigGroupGroups returns the config group snapshot taken at
// authentication time, re-filtered against current permissions.
func getRequestConfigGroupGroups(c *gin.Context, userGroup string) ([]string, bool) {
	value, ok := common.GetContextKey(c, constant.ContextKeyTokenConfigGroupGroups)
	if !ok {
		return nil, false
	}
	groups, ok := value.([]string)
	if !ok {
		return []string{}, true
	}
	return FilterUserSelectableGroups(userGroup, groups, 0), true
}

// ConfigGroupErrorMessageKey maps a resolve error to its i18n message key.
func ConfigGroupErrorMessageKey(err error) string {
	switch {
	case errors.Is(err, ErrConfigGroupNotFound):
		return i18n.MsgTokenConfigGroupNotFound
	case errors.Is(err, ErrConfigGroupForbidden):
		return i18n.MsgTokenConfigGroupForbidden
	default:
		return i18n.MsgTokenConfigGroupNoUsable
	}
}
