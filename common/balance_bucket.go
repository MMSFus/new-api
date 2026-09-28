package common

// 钱包余额分桶。users.quota 仍是总余额，满足
// quota = topup + aff_rebate + invite_bonus + gift + debt，
// 其中各桶 >= 0，debt <= 0（仅记录结算欠费）。
const (
	BalanceBucketTopup       = "topup"        // 充值余额：在线充值、兑换码
	BalanceBucketAffRebate   = "aff_rebate"   // 邀请返现余额：邀请人返利划转
	BalanceBucketInviteBonus = "invite_bonus" // 邀请激励余额：被邀请人注册奖励
	BalanceBucketGift        = "gift"         // 赠送余额：新用户赠送、签到、管理员赠送
	// BalanceBucketDebt 不是可配置的余额桶，只出现在扣费账本中，
	// 表示余额耗尽后结算补扣形成的欠费部分。
	BalanceBucketDebt = "debt"
)

// defaultBalanceBucketOrder 是未配置分组的默认扣费顺序：
// 邀请激励 → 邀请返现 → 赠送 → 充值，最后才用付费充值余额。
var defaultBalanceBucketOrder = []string{
	BalanceBucketInviteBonus,
	BalanceBucketAffRebate,
	BalanceBucketGift,
	BalanceBucketTopup,
}

// DefaultBalanceBucketOrder 返回默认扣费顺序的副本。
func DefaultBalanceBucketOrder() []string {
	return append([]string(nil), defaultBalanceBucketOrder...)
}

// IsBalanceBucket 判断是否为可配置的余额桶（不含 debt）。
func IsBalanceBucket(name string) bool {
	for _, bucket := range defaultBalanceBucketOrder {
		if bucket == name {
			return true
		}
	}
	return false
}

// BalanceLedgerEntry 记录一次扣费从某个桶扣走的额度。
type BalanceLedgerEntry struct {
	Bucket string `json:"bucket"`
	Amount int    `json:"amount"`
}

// BalanceLedger 按扣费发生顺序记录各桶扣减量，退款时从尾部（后进先出）
// 逐条退回原桶，保证退款与扣费路径严格对称。
type BalanceLedger []BalanceLedgerEntry

// Total 返回账本总额。
func (l BalanceLedger) Total() int {
	total := 0
	for _, entry := range l {
		total += entry.Amount
	}
	return total
}

// Add 追加一条扣减记录，与尾部同桶记录合并。
func (l *BalanceLedger) Add(bucket string, amount int) {
	if amount <= 0 {
		return
	}
	if n := len(*l); n > 0 && (*l)[n-1].Bucket == bucket {
		(*l)[n-1].Amount += amount
		return
	}
	*l = append(*l, BalanceLedgerEntry{Bucket: bucket, Amount: amount})
}

// Append 依序追加另一本账。
func (l *BalanceLedger) Append(other BalanceLedger) {
	for _, entry := range other {
		l.Add(entry.Bucket, entry.Amount)
	}
}

// Clone 返回深拷贝。
func (l BalanceLedger) Clone() BalanceLedger {
	if l == nil {
		return nil
	}
	return append(BalanceLedger(nil), l...)
}

// PopTail 从尾部移除最多 amount 的额度，返回被移除的记录（按移除顺序，
// 即最后扣的最先退）以及账本不足时未能覆盖的剩余额度。
func (l *BalanceLedger) PopTail(amount int) (removed BalanceLedger, remaining int) {
	remaining = amount
	for remaining > 0 && len(*l) > 0 {
		last := len(*l) - 1
		entry := (*l)[last]
		take := min(entry.Amount, remaining)
		removed = append(removed, BalanceLedgerEntry{Bucket: entry.Bucket, Amount: take})
		remaining -= take
		if take == entry.Amount {
			*l = (*l)[:last]
		} else {
			(*l)[last].Amount -= take
		}
	}
	return removed, remaining
}

// TrimTo 从尾部截断账本，使总额不超过 target（用于与持久化额度对齐）。
func (l *BalanceLedger) TrimTo(target int) {
	if target < 0 {
		target = 0
	}
	if excess := l.Total() - target; excess > 0 {
		l.PopTail(excess)
	}
}
