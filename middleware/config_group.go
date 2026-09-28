package middleware

import (
	"net/http"

	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
)

// resolveTokenConfigGroup resolves a "cfg:<key>" token group against the live
// config group settings. It returns isConfigGroup=false for ordinary groups.
// When the plan was deleted, hidden from the user group, or has no usable
// member left, the request is rejected with an explicit error instead of
// silently falling back to another group (which could change billing).
func resolveTokenConfigGroup(c *gin.Context, userGroup, tokenGroup string) (service.UserConfigGroup, bool, bool) {
	key, isConfigGroup := setting.ParseConfigGroupRef(tokenGroup)
	if !isConfigGroup {
		return service.UserConfigGroup{}, false, true
	}
	resolved, err := service.ResolveUserConfigGroup(userGroup, key)
	if err != nil {
		abortWithOpenAiMessage(c, http.StatusForbidden,
			i18n.T(c, service.ConfigGroupErrorMessageKey(err), map[string]any{"Group": key}),
			types.ErrorCodeAccessDenied)
		return service.UserConfigGroup{}, true, false
	}
	return resolved, true, true
}
