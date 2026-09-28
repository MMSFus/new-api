package middleware

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	ModelRequestRateLimitCountMark        = "MRRL"
	ModelRequestRateLimitSuccessCountMark = "MRRLS"
	modelRateLimitTimeFormat              = "2006-01-02T15:04:05.000Z"

	// modelRateLimitGateKey holds the per-request *modelRateLimitGate.
	modelRateLimitGateKey = "model_request_rate_limit_gate"
	// ModelRateLimitSourceKey records which rule limited the request.
	ModelRateLimitSourceKey = "model_request_rate_limit_source"
)

// 检查Redis中的请求限制
func checkRedisRateLimit(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64) (bool, error) {
	// 如果maxCount为0，表示不限制
	if maxCount == 0 {
		return true, nil
	}

	// 获取当前计数
	length, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		return false, err
	}

	// 如果未达到限制，允许请求
	if length < int64(maxCount) {
		return true, nil
	}

	// 检查时间窗口
	oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
	oldTime, err := time.Parse(modelRateLimitTimeFormat, oldTimeStr)
	if err != nil {
		return false, err
	}

	nowTimeStr := time.Now().UTC().Format(modelRateLimitTimeFormat)
	nowTime, err := time.Parse(modelRateLimitTimeFormat, nowTimeStr)
	if err != nil {
		return false, err
	}
	// 如果在时间窗口内已达到限制，拒绝请求
	subTime := nowTime.Sub(oldTime).Seconds()
	if int64(subTime) < duration {
		rdb.Expire(ctx, key, modelRateLimitExpiry(duration))
		return false, nil
	}

	return true, nil
}

// 记录Redis请求
func recordRedisRequest(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64) {
	// 如果maxCount为0，不记录请求
	if maxCount == 0 {
		return
	}

	now := time.Now().UTC().Format(modelRateLimitTimeFormat)
	rdb.LPush(ctx, key, now)
	rdb.LTrim(ctx, key, 0, int64(maxCount-1))
	rdb.Expire(ctx, key, modelRateLimitExpiry(duration))
}

func modelRateLimitExpiry(durationSeconds int64) time.Duration {
	if durationSeconds <= 0 {
		return time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute
	}
	return time.Duration(durationSeconds) * time.Second
}

// modelRateLimitPlan is the resolved limit and counter keys for one request.
type modelRateLimitPlan struct {
	userID          string
	scope           string
	source          string
	durationSeconds int64
	totalMaxCount   int
	successMaxCount int
}

func (p modelRateLimitPlan) durationMinutes() int64 {
	return p.durationSeconds / 60
}

// modelRateLimitScopeSuffix isolates counters per called group. The legacy
// table and the default limit keep the historic per-user keys, so upgrading
// neither resets nor splits counters of existing deployments.
func (p modelRateLimitPlan) scopeSuffix() string {
	if p.scope == "" {
		return ""
	}
	return ":g:" + p.scope
}

func (p modelRateLimitPlan) redisTotalKey() string {
	return fmt.Sprintf("rateLimit:%s%s", p.userID, p.scopeSuffix())
}

func (p modelRateLimitPlan) redisSuccessKey() string {
	return fmt.Sprintf("rateLimit:%s:%s%s", ModelRequestRateLimitSuccessCountMark, p.userID, p.scopeSuffix())
}

func (p modelRateLimitPlan) memoryTotalKey() string {
	return ModelRequestRateLimitCountMark + p.userID + p.scopeSuffix()
}

func (p modelRateLimitPlan) memorySuccessKey() string {
	return ModelRequestRateLimitSuccessCountMark + p.userID + p.scopeSuffix()
}

// modelRateLimitDenial is a rejected admission. An empty message keeps the
// bare status response the in-memory limiter has always returned.
type modelRateLimitDenial struct {
	status  int
	message string
}

func (d *modelRateLimitDenial) abort(c *gin.Context) {
	if d.message == "" {
		c.AbortWithStatus(d.status)
		return
	}
	abortWithOpenAiMessage(c, d.status, d.message)
}

func (d *modelRateLimitDenial) apiError() *types.NewAPIError {
	message := d.message
	if message == "" {
		message = http.StatusText(d.status)
	}
	return types.NewErrorWithStatusCode(errors.New(message), types.ErrorCodeInvalidRequest, d.status, types.ErrOptionWithSkipRetry())
}

// admitRedis checks both limits and returns a finisher that records a
// successful request.
func admitRedis(plan modelRateLimitPlan) (*modelRateLimitDenial, func(bool)) {
	ctx := context.Background()
	rdb := common.RDB

	// 1. 检查成功请求数限制
	successKey := plan.redisSuccessKey()
	allowed, err := checkRedisRateLimit(ctx, rdb, successKey, plan.successMaxCount, plan.durationSeconds)
	if err != nil {
		fmt.Println("检查成功请求数限制失败:", err.Error())
		return &modelRateLimitDenial{status: http.StatusInternalServerError, message: "rate_limit_check_failed"}, nil
	}
	if !allowed {
		return &modelRateLimitDenial{status: http.StatusTooManyRequests, message: fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", plan.durationMinutes(), plan.successMaxCount)}, nil
	}

	//2.检查总请求数限制并记录总请求（当totalMaxCount为0时会自动跳过，使用令牌桶限流器
	if plan.totalMaxCount > 0 {
		tb := limiter.New(ctx, rdb)
		allowed, err = tb.Allow(
			ctx,
			plan.redisTotalKey(),
			limiter.WithCapacity(rateLimitCapacity(plan.totalMaxCount, plan.durationSeconds)),
			limiter.WithRate(int64(plan.totalMaxCount)),
			limiter.WithRequested(plan.durationSeconds),
		)
		if err != nil {
			fmt.Println("检查总请求数限制失败:", err.Error())
			return &modelRateLimitDenial{status: http.StatusInternalServerError, message: "rate_limit_check_failed"}, nil
		}
		if !allowed {
			return &modelRateLimitDenial{status: http.StatusTooManyRequests, message: fmt.Sprintf("您已达到总请求数限制：%d分钟内最多请求%d次，包括失败次数，请检查您的请求是否正确", plan.durationMinutes(), plan.totalMaxCount)}, nil
		}
	}

	return nil, func(success bool) {
		if success {
			recordRedisRequest(ctx, rdb, successKey, plan.successMaxCount, plan.durationSeconds)
		}
	}
}

// admitMemory checks the total limit and reserves a success slot. The
// finisher releases the reservation, recording it only on success.
func admitMemory(plan modelRateLimitPlan) (*modelRateLimitDenial, func(bool)) {
	inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)

	// 1. 检查总请求数限制（当totalMaxCount为0时跳过）
	if plan.totalMaxCount > 0 && !inMemoryRateLimiter.Request(plan.memoryTotalKey(), plan.totalMaxCount, plan.durationSeconds) {
		return &modelRateLimitDenial{status: http.StatusTooManyRequests}, nil
	}

	var reservation *common.RateLimitReservation
	if plan.successMaxCount > 0 {
		reservation = inMemoryRateLimiter.Reserve(plan.memorySuccessKey(), plan.successMaxCount, plan.durationSeconds)
		if reservation == nil {
			return &modelRateLimitDenial{status: http.StatusTooManyRequests}, nil
		}
	}
	return nil, reservation.Complete
}

func admitModelRequest(plan modelRateLimitPlan, useRedis bool) (*modelRateLimitDenial, func(bool)) {
	if useRedis {
		return admitRedis(plan)
	}
	return admitMemory(plan)
}

// runModelRateLimitPlan admits, runs the rest of the chain, then records.
func runModelRateLimitPlan(c *gin.Context, plan modelRateLimitPlan, useRedis bool) {
	denial, finish := admitModelRequest(plan, useRedis)
	if denial != nil {
		denial.abort(c)
		return
	}
	if finish != nil {
		defer finish(false)
	}
	c.Next()
	if finish != nil {
		finish(modelRequestSucceeded(c))
	}
}

func legacyModelRateLimitPlan(c *gin.Context, duration int64, totalMaxCount, successMaxCount int) modelRateLimitPlan {
	return modelRateLimitPlan{
		userID:          strconv.Itoa(c.GetInt("id")),
		durationSeconds: duration,
		totalMaxCount:   totalMaxCount,
		successMaxCount: successMaxCount,
	}
}

// Redis限流处理器
func redisRateLimitHandler(duration int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	return func(c *gin.Context) {
		runModelRateLimitPlan(c, legacyModelRateLimitPlan(c, duration, totalMaxCount, successMaxCount), true)
	}
}

// 内存限流处理器
func memoryRateLimitHandler(duration int64, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	return func(c *gin.Context) {
		runModelRateLimitPlan(c, legacyModelRateLimitPlan(c, duration, totalMaxCount, successMaxCount), false)
	}
}

func modelRequestSucceeded(c *gin.Context) bool {
	status, _ := common.GetContextKeyType[*relaycommon.StreamStatus](c, constant.ContextKeyResponseStreamStatus)
	return c.Writer.Status() < 400 && !status.ResponseFailed()
}

// modelRateLimitCalledGroup returns the group the request is served from.
// An "auto" token is resolved only once channel selection has picked a
// concrete group; until then resolved is false.
func modelRateLimitCalledGroup(c *gin.Context) (group string, resolved bool) {
	if autoGroup := common.GetContextKeyString(c, constant.ContextKeyAutoGroup); autoGroup != "" {
		return autoGroup, true
	}
	group = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	}
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}
	return group, group != "auto"
}

func resolveModelRateLimitPlan(c *gin.Context) modelRateLimitPlan {
	calledGroup, _ := modelRateLimitCalledGroup(c)
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	// The legacy table was always keyed by the token group, falling back to
	// the user group; keep that lookup for backward compatibility.
	legacyGroup := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	if legacyGroup == "" {
		legacyGroup = userGroup
	}
	rule := setting.ResolveModelRequestRateLimit(userGroup, calledGroup, legacyGroup)
	return modelRateLimitPlan{
		userID:          strconv.Itoa(c.GetInt("id")),
		scope:           rule.Scope,
		source:          rule.Source,
		durationSeconds: rateLimitDurationSeconds(rule.DurationMinutes),
		totalMaxCount:   rule.Total,
		successMaxCount: rule.Success,
	}
}

// modelRateLimitGate carries one request's admission from the point the
// called group is known to the end of the middleware chain. A request is
// admitted at most once: cross-group retries after admission stay counted
// against the first resolved group.
type modelRateLimitGate struct {
	useRedis bool
	checked  bool
	denial   *modelRateLimitDenial
	finish   func(bool)
}

func (g *modelRateLimitGate) enforce(c *gin.Context) *modelRateLimitDenial {
	if g.checked {
		return g.denial
	}
	g.checked = true
	plan := resolveModelRateLimitPlan(c)
	c.Set(ModelRateLimitSourceKey, plan.source)
	g.denial, g.finish = admitModelRequest(plan, g.useRedis)
	return g.denial
}

func (g *modelRateLimitGate) complete(success bool) {
	if g.finish == nil {
		return
	}
	finish := g.finish
	g.finish = nil
	finish(success)
}

func getModelRateLimitGate(c *gin.Context) *modelRateLimitGate {
	value, ok := c.Get(modelRateLimitGateKey)
	if !ok {
		return nil
	}
	gate, _ := value.(*modelRateLimitGate)
	return gate
}

// EnforceModelRequestRateLimit admits a request whose group was not known
// when ModelRequestRateLimit ran (an "auto" token). Call it once the group is
// selected. It aborts and returns false when the request is limited, and is
// a no-op for requests already admitted or not rate limited.
func EnforceModelRequestRateLimit(c *gin.Context) bool {
	gate := getModelRateLimitGate(c)
	if gate == nil {
		return true
	}
	if denial := gate.enforce(c); denial != nil {
		denial.abort(c)
		return false
	}
	return true
}

// CheckModelRequestRateLimit is EnforceModelRequestRateLimit for handlers
// that report errors instead of writing a gin response.
func CheckModelRequestRateLimit(c *gin.Context) *types.NewAPIError {
	gate := getModelRateLimitGate(c)
	if gate == nil {
		return nil
	}
	if denial := gate.enforce(c); denial != nil {
		return denial.apiError()
	}
	return nil
}

// ModelRequestRateLimit 模型请求限流中间件
//
// The limit is chosen by the called group: a private rule for the user's
// group calling it, else a global rule for it, else the legacy table, else
// the default. Requests with a concrete group are admitted here; "auto"
// requests are admitted by EnforceModelRequestRateLimit after Distribute
// (or the Responses WebSocket) has selected the group.
func ModelRequestRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 在每个请求时检查是否启用限流
		if !setting.ModelRequestRateLimitEnabled {
			c.Next()
			return
		}
		if getModelRateLimitGate(c) != nil {
			// Mounted twice on one chain: the outer instance owns admission.
			c.Next()
			return
		}

		gate := &modelRateLimitGate{useRedis: common.RedisEnabled}
		c.Set(modelRateLimitGateKey, gate)
		defer gate.complete(false)

		if _, resolved := modelRateLimitCalledGroup(c); resolved {
			if denial := gate.enforce(c); denial != nil {
				denial.abort(c)
				return
			}
		}

		c.Next()
		gate.complete(modelRequestSucceeded(c))
	}
}

func rateLimitDurationSeconds(durationMinutes int) int64 {
	if durationMinutes <= 0 {
		return 0
	}
	minutes := int64(durationMinutes)
	if minutes > math.MaxInt64/60 {
		return math.MaxInt64
	}
	return minutes * 60
}

func rateLimitCapacity(count int, durationSeconds int64) int64 {
	if count <= 0 || durationSeconds <= 0 {
		return 0
	}
	c := int64(count)
	if c > math.MaxInt64/durationSeconds {
		return math.MaxInt64
	}
	return c * durationSeconds
}
