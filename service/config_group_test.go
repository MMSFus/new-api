package service

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configureConfigGroupsTest(t *testing.T, configJSON string) {
	t.Helper()
	originalConfig := setting.ConfigGroups2JSONString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalRatios := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP","svip":"SVIP"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2,"svip":3}`))
	require.NoError(t, setting.UpdateConfigGroupsByJSONString(configJSON))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateConfigGroupsByJSONString(originalConfig))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatios))
	})
}

func TestParseConfigGroupsNormalizesAndRejectsInvalidPlans(t *testing.T) {
	list, err := setting.ParseConfigGroups(`[{"key":" best ","name":" Best ","groups":[" vip ","default"],"user_groups":["vip","vip"],"cross_group_retry":true}]`)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, setting.ConfigGroup{
		Key: "best", Name: "Best", Groups: []string{"vip", "default"},
		UserGroups: []string{"vip"}, CrossGroupRetry: true,
	}, list[0])

	empty, err := setting.ParseConfigGroups("")
	require.NoError(t, err)
	assert.Empty(t, empty)

	for name, value := range map[string]string{
		"not array":       `{"key":"a"}`,
		"missing key":     `[{"groups":["vip"]}]`,
		"reserved key":    `[{"key":"Auto","groups":["vip"]}]`,
		"key with colon":  `[{"key":"cfg:a","groups":["vip"]}]`,
		"key with space":  `[{"key":"a b","groups":["vip"]}]`,
		"key too long":    `[{"key":"abcdefghijklmnopqrstuvwxyz0123456","groups":["vip"]}]`,
		"duplicate key":   `[{"key":"a","groups":["vip"]},{"key":"a","groups":["vip"]}]`,
		"no groups":       `[{"key":"a","groups":[]}]`,
		"duplicate group": `[{"key":"a","groups":["vip","vip"]}]`,
		"auto member":     `[{"key":"a","groups":["auto"]}]`,
		"nested ref":      `[{"key":"a","groups":["cfg:b"]}]`,
		"too many groups": `[{"key":"a","groups":["1","2","3","4","5","6","7","8","9","10","11","12","13","14","15","16","17","18","19","20","21"]}]`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := setting.ParseConfigGroups(value)
			assert.Error(t, err)
		})
	}
}

func TestValidateConfigGroupsJSONChecksGroupsAgainstRatios(t *testing.T) {
	configureConfigGroupsTest(t, `[]`)

	assert.NoError(t, setting.ValidateConfigGroupsJSON(`[{"key":"best","groups":["vip","default"]}]`))
	assert.ErrorContains(t, setting.ValidateConfigGroupsJSON(`[{"key":"best","groups":["vip","gone"]}]`), "unknown group")
	assert.ErrorContains(t, setting.ValidateConfigGroupsJSON(`[{"key":"vip","groups":["default"]}]`), "conflicts")
}

func TestUpdateConfigGroupsKeepsPreviousSnapshotOnError(t *testing.T) {
	configureConfigGroupsTest(t, `[{"key":"best","groups":["vip"]}]`)

	require.Error(t, setting.UpdateConfigGroupsByJSONString(`not-json`))
	cfg, ok := setting.GetConfigGroup("best")
	require.True(t, ok)
	assert.Equal(t, []string{"vip"}, cfg.Groups)

	cfg.Groups[0] = "mutated"
	again, _ := setting.GetConfigGroup("best")
	assert.Equal(t, []string{"vip"}, again.Groups, "callers must receive a copy")
}

func TestResolveUserConfigGroupFiltersByUserPermissions(t *testing.T) {
	configureConfigGroupsTest(t, `[
		{"key":"best","name":"Best","groups":["svip","vip","default"],"cross_group_retry":true},
		{"key":"vip-only","groups":["vip"],"user_groups":["vip"]},
		{"key":"svip-only","groups":["svip"]}
	]`)
	// default users may use default and vip, but not svip.
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))

	resolved, err := ResolveUserConfigGroup("default", "best")
	require.NoError(t, err)
	assert.Equal(t, UserConfigGroup{
		Key: "best", Ref: "cfg:best", Name: "Best",
		Groups: []string{"vip", "default"}, CrossGroupRetry: true,
	}, resolved)

	_, err = ResolveUserConfigGroup("default", "vip-only")
	assert.ErrorIs(t, err, ErrConfigGroupForbidden)
	_, err = ResolveUserConfigGroup("default", "svip-only")
	assert.ErrorIs(t, err, ErrConfigGroupNoUsableGroups)

	visible := GetUserConfigGroups("default")
	require.Len(t, visible, 1)
	assert.Equal(t, "best", visible[0].Key)
	assert.Len(t, GetUserConfigGroups("vip"), 2)
}

func TestResolveUserConfigGroupAfterDeletionReportsNotFound(t *testing.T) {
	configureConfigGroupsTest(t, `[{"key":"best","groups":["vip"]}]`)
	_, err := ResolveUserConfigGroup("default", "best")
	require.NoError(t, err)

	require.NoError(t, setting.UpdateConfigGroupsByJSONString(`[]`))

	_, err = ResolveUserConfigGroup("default", "best")
	assert.ErrorIs(t, err, ErrConfigGroupNotFound)
	assert.Equal(t, i18n.MsgTokenConfigGroupNotFound, ConfigGroupErrorMessageKey(err))
}

func TestGetRequestAutoGroupsUsesConfigGroupOrderWithoutTokenLimit(t *testing.T) {
	configureConfigGroupsTest(t, `[{"key":"best","groups":["svip","vip","default"]}]`)
	originalMax := setting.GetMaxTokenAutoGroups()
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("1"))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateMaxTokenAutoGroups(strconv.Itoa(originalMax)))
	})

	resolved, err := ResolveUserConfigGroup("default", "best")
	require.NoError(t, err)
	ctx := newRequestAutoGroupsContext()
	// A stale per-token list must not leak into a config group request.
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"default"})
	ApplyConfigGroupContext(ctx, resolved)

	assert.Equal(t, "auto", common.GetContextKeyString(ctx, constant.ContextKeyUsingGroup))
	assert.Equal(t, "auto", common.GetContextKeyString(ctx, constant.ContextKeyTokenGroup))
	assert.Equal(t, "best", common.GetContextKeyString(ctx, constant.ContextKeyTokenConfigGroup))
	assert.Equal(t, []string{"svip", "vip", "default"}, GetRequestAutoGroups(ctx, "default"))

	// Permissions revoked mid-flight are re-applied on every lookup.
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default"}`))
	assert.Equal(t, []string{"default"}, GetRequestAutoGroups(ctx, "default"))
}

func TestCacheGetRandomSatisfiedChannelFailsOverInConfigGroupOrder(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	configureConfigGroupsTest(t, `[{"key":"best","groups":["svip","vip","default"]}]`)
	const modelName = "config-group-runtime-model"
	// svip has no channel for the model, so selection starts at vip.
	createChannelSelectAutoGroupsChannel(t, db, 2201, "vip", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2202, "default", modelName)
	model.InitChannelCache()

	resolved, err := ResolveUserConfigGroup("default", "best")
	require.NoError(t, err)
	ctx := newRequestAutoGroupsContext()
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
	ApplyConfigGroupContext(ctx, resolved)

	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  common.GetContextKeyString(ctx, constant.ContextKeyTokenGroup),
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}

	first, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2201, first.Id)
	assert.Equal(t, "vip", selectedGroup)
	assert.Equal(t, "vip", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup),
		"billing and logs use the real group that served the request")

	param.IncreaseRetry()
	second, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2202, second.Id)
	assert.Equal(t, "default", selectedGroup)
}
