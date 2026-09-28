package middleware

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// useGroupRateLimitSettings enables model rate limiting with the given
// tables for one test and restores the previous settings afterwards.
func useGroupRateLimitSettings(t *testing.T, legacy, global, private string) {
	t.Helper()
	prevEnabled := setting.ModelRequestRateLimitEnabled
	prevDuration := setting.ModelRequestRateLimitDurationMinutes
	prevCount := setting.ModelRequestRateLimitCount
	prevSuccess := setting.ModelRequestRateLimitSuccessCount
	prevLegacy := setting.ModelRequestRateLimitGroup2JSONString()
	prevGlobal := setting.ModelRequestRateLimitGlobalGroup2JSONString()
	prevPrivate := setting.ModelRequestRateLimitPrivateGroup2JSONString()
	t.Cleanup(func() {
		setting.ModelRequestRateLimitEnabled = prevEnabled
		setting.ModelRequestRateLimitDurationMinutes = prevDuration
		setting.ModelRequestRateLimitCount = prevCount
		setting.ModelRequestRateLimitSuccessCount = prevSuccess
		require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(prevLegacy))
		require.NoError(t, setting.UpdateModelRequestRateLimitGlobalGroupByJSONString(prevGlobal))
		require.NoError(t, setting.UpdateModelRequestRateLimitPrivateGroupByJSONString(prevPrivate))
	})
	setting.ModelRequestRateLimitEnabled = true
	setting.ModelRequestRateLimitDurationMinutes = 1
	setting.ModelRequestRateLimitCount = 0
	setting.ModelRequestRateLimitSuccessCount = 100
	require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(legacy))
	require.NoError(t, setting.UpdateModelRequestRateLimitGlobalGroupByJSONString(global))
	require.NoError(t, setting.UpdateModelRequestRateLimitPrivateGroupByJSONString(private))
}

func useModelRateLimitBackend(t *testing.T, backend string) {
	t.Helper()
	if backend == "redis" {
		useRateLimitMiniRedis(t)
		return
	}
	previous := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previous })
}

// groupRateLimitRouter serves /:userGroup/:tokenGroup/:autoGroup. The token
// group is also the using group, as TokenAuth sets it; "-" means empty.
// When resolve is true a Distribute stand-in sets the auto group and admits
// the request, as Distribute does after channel selection.
func groupRateLimitRouter(userID int, resolve bool, handled *int) *gin.Engine {
	param := func(c *gin.Context, name string) string {
		if v := c.Param(name); v != "-" {
			return v
		}
		return ""
	}
	router := gin.New()
	router.GET("/:userGroup/:tokenGroup/:autoGroup", func(c *gin.Context) {
		c.Set("id", userID)
		userGroup, tokenGroup := param(c, "userGroup"), param(c, "tokenGroup")
		common.SetContextKey(c, constant.ContextKeyUserGroup, userGroup)
		common.SetContextKey(c, constant.ContextKeyTokenGroup, tokenGroup)
		usingGroup := tokenGroup
		if usingGroup == "" {
			usingGroup = userGroup
		}
		common.SetContextKey(c, constant.ContextKeyUsingGroup, usingGroup)
	}, ModelRequestRateLimit(), func(c *gin.Context) {
		if !resolve {
			return
		}
		if autoGroup := param(c, "autoGroup"); autoGroup != "" {
			common.SetContextKey(c, constant.ContextKeyAutoGroup, autoGroup)
		}
		if !EnforceModelRequestRateLimit(c) {
			return
		}
		// A cross-group retry switches groups and re-enters admission; it
		// must neither re-check nor count again.
		common.SetContextKey(c, constant.ContextKeyAutoGroup, "retry-group")
		if !EnforceModelRequestRateLimit(c) {
			return
		}
	}, func(c *gin.Context) {
		*handled++
		c.Status(http.StatusOK)
	})
	return router
}

func TestModelRateLimitGroupRulesPerBackend(t *testing.T) {
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			useModelRateLimitBackend(t, backend)
			useGroupRateLimitSettings(t,
				`{"vip":[0,3]}`,
				`{"claude":{"total":0,"success":2}}`,
				`{"vip":{"claude":{"total":0,"success":1}}}`,
			)
			handled := 0
			get := func(userID int, path string) int {
				return performRateLimitRequest(groupRateLimitRouter(userID, true, &handled), path, "127.0.0.1:1000").Code
			}

			// Private rule: vip calling claude gets 1 success.
			vipUser := 7300000 + int(modelRateLimitTestUsers.Add(1))
			assert.Equal(t, http.StatusOK, get(vipUser, "/vip/claude/-"))
			assert.Equal(t, http.StatusTooManyRequests, get(vipUser, "/vip/claude/-"))

			// The same user calling another group has its own counter; vip
			// has no private rule for gpt, so the legacy vip entry (keyed by
			// the token group, falling back to the user group) applies.
			assert.Equal(t, http.StatusOK, get(vipUser, "/vip/-/-"))
			assert.Equal(t, http.StatusOK, get(vipUser, "/vip/-/-"))
			assert.Equal(t, http.StatusOK, get(vipUser, "/vip/-/-"))
			assert.Equal(t, http.StatusTooManyRequests, get(vipUser, "/vip/-/-"))

			// Global rule: any other user calling claude gets 2 successes.
			defaultUser := 7300000 + int(modelRateLimitTestUsers.Add(1))
			assert.Equal(t, http.StatusOK, get(defaultUser, "/default/claude/-"))
			assert.Equal(t, http.StatusOK, get(defaultUser, "/default/claude/-"))
			assert.Equal(t, http.StatusTooManyRequests, get(defaultUser, "/default/claude/-"))

			// An auto token is admitted against the group selected for it,
			// and the simulated cross-group retry is not counted again.
			autoUser := 7300000 + int(modelRateLimitTestUsers.Add(1))
			assert.Equal(t, http.StatusOK, get(autoUser, "/default/auto/claude"))
			assert.Equal(t, http.StatusOK, get(autoUser, "/default/auto/claude"))
			assert.Equal(t, http.StatusTooManyRequests, get(autoUser, "/default/auto/claude"))
			// Its default-limited traffic (no rule for gpt) is unaffected.
			assert.Equal(t, http.StatusOK, get(autoUser, "/default/auto/gpt"))
		})
	}
}

func TestModelRateLimitPrivateWildcardAndCounterKeys(t *testing.T) {
	redisServer, _ := useRateLimitMiniRedis(t)
	useGroupRateLimitSettings(t, `{}`,
		`{"claude":{"total":5,"success":5}}`,
		`{"vip":{"*":{"total":0,"success":1,"duration":10}}}`,
	)
	handled := 0
	userID := 7300000 + int(modelRateLimitTestUsers.Add(1))
	get := func(path string) int {
		return performRateLimitRequest(groupRateLimitRouter(userID, true, &handled), path, "127.0.0.1:1000").Code
	}
	// The wildcard beats the global claude rule and is shared by every
	// group this vip user calls.
	assert.Equal(t, http.StatusOK, get("/vip/claude/-"))
	assert.Equal(t, http.StatusTooManyRequests, get("/vip/gpt/-"))
	id := fmt.Sprint(userID)
	assert.True(t, redisServer.Exists("rateLimit:MRRLS:"+id+":g:*"))
	assert.False(t, redisServer.Exists("rateLimit:MRRLS:"+id+":g:claude"))
	assert.False(t, redisServer.Exists("rateLimit:MRRLS:"+id), "rule scoped counters stay apart from the per-user default key")
	ttl := redisServer.TTL("rateLimit:MRRLS:" + id + ":g:*")
	assert.Greater(t, ttl.Minutes(), 9.0, "the rule duration sets the window")

	// A default-limited user keeps the historic per-user key.
	other := 7300000 + int(modelRateLimitTestUsers.Add(1))
	assert.Equal(t, http.StatusOK, performRateLimitRequest(groupRateLimitRouter(other, true, &handled), "/default/-/-", "127.0.0.1:1000").Code)
	assert.True(t, redisServer.Exists(fmt.Sprintf("rateLimit:MRRLS:%d", other)))
}

func TestModelRateLimitAutoGroupWaitsForSelection(t *testing.T) {
	useModelRateLimitBackend(t, "memory")
	useGroupRateLimitSettings(t, `{}`, `{"claude":{"total":1,"success":0}}`, `{}`)
	handled := 0
	userID := 7300000 + int(modelRateLimitTestUsers.Add(1))
	get := func(path string) int {
		return performRateLimitRequest(groupRateLimitRouter(userID, true, &handled), path, "127.0.0.1:1000").Code
	}
	assert.Equal(t, http.StatusOK, get("/default/auto/claude"))
	assert.Equal(t, http.StatusTooManyRequests, get("/default/auto/claude"))
	assert.Equal(t, 1, handled)

	// A concrete token group is admitted before the rest of the chain runs,
	// so it is limited even on routes without Distribute.
	concrete := 7300000 + int(modelRateLimitTestUsers.Add(1))
	noResolve := groupRateLimitRouter(concrete, false, &handled)
	assert.Equal(t, http.StatusOK, performRateLimitRequest(noResolve, "/default/claude/-", "127.0.0.1:1000").Code)
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(noResolve, "/default/claude/-", "127.0.0.1:1000").Code)
}

func TestModelRateLimitDisabledSkipsGate(t *testing.T) {
	useModelRateLimitBackend(t, "memory")
	useGroupRateLimitSettings(t, `{}`, `{"claude":{"total":1,"success":1}}`, `{}`)
	setting.ModelRequestRateLimitEnabled = false
	handled := 0
	router := groupRateLimitRouter(7300000+int(modelRateLimitTestUsers.Add(1)), true, &handled)
	for i := 0; i < 3; i++ {
		assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/default/claude/-", "127.0.0.1:1000").Code)
	}
}
