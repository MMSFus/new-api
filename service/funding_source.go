package service

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

// ResolveWalletBillingGroup 返回钱包计费实际使用的分组，决定可用余额桶。
// auto 令牌与配置分组令牌的 UsingGroup 为 "auto"，必须取 Distribute 选定的
// 真实分组，否则会落到 "*"/全部余额，绕过受限分组的余额限制。
// 顺序与限速一致：AutoGroup → UsingGroup → TokenGroup → UserGroup，跳过 "auto"。
func ResolveWalletBillingGroup(c *gin.Context, info *relaycommon.RelayInfo) string {
	var candidates []string
	if c != nil {
		candidates = append(candidates, common.GetContextKeyString(c, constant.ContextKeyAutoGroup))
	}
	if info != nil {
		candidates = append(candidates, info.UsingGroup, info.TokenGroup, info.UserGroup)
	}
	if c != nil {
		candidates = append(candidates,
			common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
			common.GetContextKeyString(c, constant.ContextKeyTokenGroup),
			common.GetContextKeyString(c, constant.ContextKeyUserGroup))
	}
	for _, group := range candidates {
		if isConcreteBillingGroup(group) {
			return group
		}
	}
	return ""
}

func isConcreteBillingGroup(group string) bool {
	return group != "" && group != "auto"
}

// groupBalanceInsufficientError 是按用户语言渲染的分组余额不足错误。
// Unwrap 保留原始明细，errors.Is(err, model.ErrInsufficientGroupBalance) 仍成立。
type groupBalanceInsufficientError struct {
	message string
	detail  *model.GroupBalanceInsufficientError
}

func (e *groupBalanceInsufficientError) Error() string { return e.message }

func (e *groupBalanceInsufficientError) Unwrap() error { return e.detail }

// NewGroupBalanceInsufficientAPIError 以 lang 本地化分组余额不足错误（余额类型
// 显示为本地化名称），并保持 403 + insufficient_user_quota + 不重试的语义。
func NewGroupBalanceInsufficientAPIError(lang string, detail *model.GroupBalanceInsufficientError) *types.NewAPIError {
	group := detail.Group
	if group == "" {
		group = "default"
	}
	names := make([]string, 0, len(detail.Buckets))
	for _, bucket := range detail.Buckets {
		key := i18n.MsgBalanceBucketPrefix + bucket
		name := i18n.Translate(lang, key)
		if name == key {
			name = bucket
		}
		names = append(names, name)
	}
	message := i18n.Translate(lang, i18n.MsgBalanceGroupInsufficient, map[string]any{
		"Group":     group,
		"Buckets":   strings.Join(names, i18n.Translate(lang, i18n.MsgBalanceBucketSeparator)),
		"Available": logger.FormatQuota(detail.Available),
		"Required":  logger.FormatQuota(detail.Required),
	})
	return types.NewErrorWithStatusCode(&groupBalanceInsufficientError{message: message, detail: detail},
		types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
		types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
}

// ---------------------------------------------------------------------------
// FundingSource — 资金来源接口（钱包 or 订阅）
// ---------------------------------------------------------------------------

// FundingSource 抽象了预扣费的资金来源。
type FundingSource interface {
	// Source 返回资金来源标识："wallet" 或 "subscription"
	Source() string
	// PreConsume 从该资金来源预扣 amount 额度
	PreConsume(amount int) error
	// Settle 根据差额调整资金来源（正数补扣，负数退还）
	Settle(delta int) error
	// Refund 退还所有预扣费
	Refund() error
}

// ---------------------------------------------------------------------------
// WalletFunding — 钱包资金来源实现
// ---------------------------------------------------------------------------

// ErrInsufficientWalletQuota 钱包原子预扣失败（余额不足），未发生任何扣减。
// BillingSession 据此映射为 ErrorCodeInsufficientUserQuota，
// 使 wallet_first 等计费偏好可以回退到订阅。
var ErrInsufficientWalletQuota = errors.New("wallet quota insufficient")

// WalletFunding 从钱包扣费。group 决定可用余额桶及扣费顺序；ledger 按扣费
// 顺序记录各桶扣减量，退款与负差额结算从尾部逐条退回原桶。
type WalletFunding struct {
	userId    int
	group     string
	consumed  int                                  // 实际预扣的用户额度
	available int                                  // 创建会话时分组可用余额（信任额度判断用）
	ledger    common.BalanceLedger                 // 各余额桶扣减明细
	groupErr  *model.GroupBalanceInsufficientError // 最近一次分组余额不足的明细
}

func (w *WalletFunding) Source() string { return BillingSourceWallet }

// Ledger 返回当前扣费明细的副本。
func (w *WalletFunding) Ledger() common.BalanceLedger { return w.ledger.Clone() }

func (w *WalletFunding) PreConsume(amount int) error {
	if amount <= 0 {
		return nil
	}
	ledger, err := model.ReserveUserBalance(w.userId, w.group, amount)
	if errors.Is(err, model.ErrInsufficientGroupBalance) {
		errors.As(err, &w.groupErr)
		return ErrInsufficientWalletQuota
	}
	if err != nil {
		return err
	}
	w.consumed += amount
	w.ledger.Append(ledger)
	return nil
}

// overdraft 无条件扣减 amount：分组可用余额不足的部分记为欠费（与旧版余额可为负一致）。
func (w *WalletFunding) overdraft(amount int) error {
	ledger, err := model.DebitUserBalance(w.userId, w.group, amount)
	if err != nil {
		return err
	}
	w.consumed += amount
	w.ledger.Append(ledger)
	return nil
}

// release 按账本尾部退还 amount，账本不足的部分按分组规则退还。
func (w *WalletFunding) release(amount int) error {
	remaining := w.ledger.Clone()
	if err := model.RefundUserBalanceWithLedger(w.userId, w.group, &remaining, amount); err != nil {
		return err
	}
	w.ledger = remaining
	w.consumed -= amount
	return nil
}

// switchGroup 在跨分组重试选中新分组后迁移钱包扣费：先按账本把已扣额度
// 原样退回原余额桶，再在新分组允许的余额桶中重新扣减，不足部分记欠费
// （与结算补扣语义一致）。新分组永远不会动用它不被允许的余额桶。
// group 为空或 "auto"（尚未确定真实分组）时保持原分组。
func (w *WalletFunding) switchGroup(group string) error {
	if !isConcreteBillingGroup(group) || group == w.group {
		return nil
	}
	amount := w.consumed
	if amount > 0 {
		if err := w.release(amount); err != nil {
			return err
		}
	}
	w.group = group
	if amount > 0 {
		return w.overdraft(amount)
	}
	return nil
}

func (w *WalletFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		return w.overdraft(delta)
	}
	return w.release(-delta)
}

func (w *WalletFunding) Refund() error {
	if w.consumed <= 0 {
		return nil
	}
	// 退款是非幂等的入账操作，不能重试，否则会多退额度。
	// 订阅的 RefundSubscriptionPreConsume 有 requestId 幂等保护所以可以重试。
	return w.release(w.consumed)
}

// ---------------------------------------------------------------------------
// SubscriptionFunding — 订阅资金来源实现
// ---------------------------------------------------------------------------

type SubscriptionFunding struct {
	requestId      string
	userId         int
	modelName      string
	amount         int64 // 预扣的订阅额度（subConsume）
	subscriptionId int
	preConsumed    int64
	// 以下字段在 PreConsume 成功后填充，供 RelayInfo 同步使用
	AmountTotal     int64
	AmountUsedAfter int64
	PlanId          int
	PlanTitle       string
}

func (s *SubscriptionFunding) Source() string { return BillingSourceSubscription }

func (s *SubscriptionFunding) PreConsume(_ int) error {
	// amount 参数被忽略，使用内部 s.amount（已在构造时根据 preConsumedQuota 计算）
	res, err := model.PreConsumeUserSubscription(s.requestId, s.userId, s.modelName, 0, s.amount)
	if err != nil {
		return err
	}
	s.subscriptionId = res.UserSubscriptionId
	s.preConsumed = res.PreConsumed
	s.AmountTotal = res.AmountTotal
	s.AmountUsedAfter = res.AmountUsedAfter
	// 获取订阅计划信息
	if planInfo, err := model.GetSubscriptionPlanInfoByUserSubscriptionId(res.UserSubscriptionId); err == nil && planInfo != nil {
		s.PlanId = planInfo.PlanId
		s.PlanTitle = planInfo.PlanTitle
	}
	return nil
}

func (s *SubscriptionFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	return model.PostConsumeUserSubscriptionDelta(s.subscriptionId, int64(delta))
}

func (s *SubscriptionFunding) Refund() error {
	if s.preConsumed <= 0 {
		return nil
	}
	return refundWithRetry(func() error {
		return model.RefundSubscriptionPreConsume(s.requestId)
	})
}

// refundWithRetry 尝试多次执行退款操作以提高成功率，只能用于基于事务的退款函数！！！！！！
// try to refund with retries, only for refund functions based on transactions!!!
func refundWithRetry(fn func() error) error {
	if fn == nil {
		return nil
	}
	const maxAttempts = 3
	var lastErr error
	for i := range maxAttempts {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i < maxAttempts-1 {
			time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
		}
	}
	return lastErr
}
