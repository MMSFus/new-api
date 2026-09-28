package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTokenAutoGroupsContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	return ctx
}

func TestSetupContextForTokenPreservesCustomAutoGroupsOrder(t *testing.T) {
	ctx := newTokenAutoGroupsContext()
	token := &model.Token{Id: 1, UserId: 2, AutoGroups: `["vip","default"]`}

	require.NoError(t, SetupContextForToken(ctx, token))
	value, ok := common.GetContextKey(ctx, constant.ContextKeyTokenAutoGroups)
	require.True(t, ok)
	assert.Equal(t, []string{"vip", "default"}, value)
}

func TestSetupContextForTokenTreatsStoredEmptyArrayAsInheritance(t *testing.T) {
	ctx := newTokenAutoGroupsContext()
	token := &model.Token{Id: 1, UserId: 2, AutoGroups: `[]`}

	require.NoError(t, SetupContextForToken(ctx, token))
	_, ok := common.GetContextKey(ctx, constant.ContextKeyTokenAutoGroups)
	assert.False(t, ok)
}

func TestSetupContextForTokenMalformedAutoGroupsFailsClosed(t *testing.T) {
	ctx := newTokenAutoGroupsContext()
	token := &model.Token{Id: 1, UserId: 2, AutoGroups: `not-json`}

	require.NoError(t, SetupContextForToken(ctx, token))
	value, ok := common.GetContextKey(ctx, constant.ContextKeyTokenAutoGroups)
	require.True(t, ok)
	assert.Equal(t, []string{}, value)
}

func TestResolveTokenConfigGroupRejectsDeletedPlanExplicitly(t *testing.T) {
	require.NoError(t, i18n.Init())
	originalConfig := setting.ConfigGroups2JSONString()
	originalRatios := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	require.NoError(t, setting.UpdateConfigGroupsByJSONString(`[{"key":"best","groups":["default"]}]`))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateConfigGroupsByJSONString(originalConfig))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalRatios))
	})

	ctx := newTokenAutoGroupsContext()
	resolved, isConfigGroup, ok := resolveTokenConfigGroup(ctx, "default", "cfg:best")
	require.True(t, ok)
	require.True(t, isConfigGroup)
	assert.Equal(t, []string{"default"}, resolved.Groups)

	_, isConfigGroup, ok = resolveTokenConfigGroup(ctx, "default", "default")
	assert.True(t, ok)
	assert.False(t, isConfigGroup)

	require.NoError(t, setting.UpdateConfigGroupsByJSONString(`[]`))
	recorder := httptest.NewRecorder()
	gin.SetMode(gin.TestMode)
	deletedCtx, _ := gin.CreateTestContext(recorder)
	deletedCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	_, isConfigGroup, ok = resolveTokenConfigGroup(deletedCtx, "default", "cfg:best")
	assert.False(t, ok)
	assert.True(t, isConfigGroup)
	assert.True(t, deletedCtx.IsAborted())
	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "best")
}
