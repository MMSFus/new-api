package model

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/setting"

	"gorm.io/gorm"
)

// 余额分桶设计要点：
//   - users.quota 仍是总余额，所有读取总额的代码与 Redis 缓存保持不变；
//   - 各桶列与 quota 在同一条条件 UPDATE 中写入（比较并交换），数据库中
//     始终满足 quota = 各桶之和 + debt；
//   - 钱包增减一律直写数据库（不再进入 BatchUpdate 队列），提交后再把总额
//     增量同步到 Redis 缓存，因此数据库是余额的唯一权威来源；
//   - 读取时若发现 quota 与桶之和不一致（旧版本节点或遗漏路径直接改了
//     quota），差额在下一次写入时自愈：正差计入赠送余额（来源未知的钱
//     绝不计入权限最高的充值余额），负差按默认顺序扣减。

var (
	// ErrInsufficientGroupBalance 分组可用余额不足（预扣严格校验）。
	ErrInsufficientGroupBalance = errors.New("insufficient group balance")
	// ErrInvalidBalanceBucket 未知余额类型。
	ErrInvalidBalanceBucket = errors.New("invalid balance bucket")
	// ErrBalanceBucketInsufficient 管理员扣减的额度超过该余额桶的余额。
	ErrBalanceBucketInsufficient = errors.New("balance bucket insufficient")

	errBalanceConflict = errors.New("user balance update conflict")
)

const maxBalanceUpdateAttempts = 8

// GroupBalanceInsufficientError 携带分组可用余额明细，用于返回明确的错误信息。
type GroupBalanceInsufficientError struct {
	Group     string
	Buckets   []string
	Available int
	Required  int
}

func (e *GroupBalanceInsufficientError) Error() string {
	group := e.Group
	if group == "" {
		group = "default"
	}
	return fmt.Sprintf("分组 %s 可用余额不足（可用余额类型: %s），可用 %s，需要 %s",
		group, strings.Join(e.Buckets, ", "), logger.FormatQuota(e.Available), logger.FormatQuota(e.Required))
}

func (e *GroupBalanceInsufficientError) Is(target error) bool {
	return target == ErrInsufficientGroupBalance
}

// BalanceBuckets 是用户余额的分桶快照。
type BalanceBuckets struct {
	Topup       int `json:"topup"`
	AffRebate   int `json:"aff_rebate"`
	InviteBonus int `json:"invite_bonus"`
	Gift        int `json:"gift"`
	Debt        int `json:"debt"`
}

var balanceBucketColumns = []string{"quota", "quota_topup", "quota_aff_rebate", "quota_invite_bonus", "quota_gift", "quota_debt"}

func (b *BalanceBuckets) ptr(bucket string) *int {
	switch bucket {
	case common.BalanceBucketTopup:
		return &b.Topup
	case common.BalanceBucketAffRebate:
		return &b.AffRebate
	case common.BalanceBucketInviteBonus:
		return &b.InviteBonus
	case common.BalanceBucketGift:
		return &b.Gift
	case common.BalanceBucketDebt:
		return &b.Debt
	}
	return nil
}

// Get 返回某个桶的余额。
func (b BalanceBuckets) Get(bucket string) int {
	if p := b.ptr(bucket); p != nil {
		return *p
	}
	return 0
}

// Total 返回各桶之和（含欠费），应等于 users.quota。
func (b BalanceBuckets) Total() int {
	return b.Topup + b.AffRebate + b.InviteBonus + b.Gift + b.Debt
}

// Available 返回给定桶列表的可用余额之和。
func (b BalanceBuckets) Available(buckets []string) int {
	total := 0
	for _, bucket := range buckets {
		if bucket == common.BalanceBucketDebt {
			continue
		}
		if v := b.Get(bucket); v > 0 {
			total += v
		}
	}
	return total
}

// credit 把 amount 计入某个桶；存在欠费时先偿还欠费。
// 对 debt 本身入账表示退回结算欠费，超出欠费的部分计入赠送余额。
func (b *BalanceBuckets) credit(bucket string, amount int) {
	if amount <= 0 {
		return
	}
	if bucket == common.BalanceBucketDebt {
		b.Debt += amount
		if b.Debt > 0 {
			b.Gift += b.Debt
			b.Debt = 0
		}
		return
	}
	if b.Debt < 0 {
		repay := min(amount, -b.Debt)
		b.Debt += repay
		amount -= repay
	}
	*b.ptr(bucket) += amount
}

// debit 按 order 依次扣减，返回扣费账本与未能覆盖的额度。
// overdraft=true 时（结算补扣，服务已交付）未能覆盖的部分记为欠费，总是全额扣减；
// 欠费不会动用 order 之外的余额桶，保证分组余额隔离，之后的任意入账优先偿还欠费。
func (b *BalanceBuckets) debit(order []string, amount int, overdraft bool) (common.BalanceLedger, int) {
	var ledger common.BalanceLedger
	remaining := amount
	for _, bucket := range order {
		p := b.ptr(bucket)
		if p == nil || bucket == common.BalanceBucketDebt || *p <= 0 || remaining <= 0 {
			continue
		}
		n := min(*p, remaining)
		*p -= n
		remaining -= n
		ledger.Add(bucket, n)
	}
	if overdraft && remaining > 0 {
		b.Debt -= remaining
		ledger.Add(common.BalanceBucketDebt, remaining)
		remaining = 0
	}
	return ledger, remaining
}

// normalize 修复负数桶以及 quota 与桶之和的偏差：正差来源未知，计入赠送
// 余额（充值余额可被分组设为专用，不能凭空获得）；负差按默认顺序扣减、
// 不足记欠费。
func (b BalanceBuckets) normalize(quota int) BalanceBuckets {
	if b.Debt > 0 {
		b.Gift += b.Debt
		b.Debt = 0
	}
	for _, bucket := range common.DefaultBalanceBucketOrder() {
		if p := b.ptr(bucket); *p < 0 {
			b.Debt += *p
			*p = 0
		}
	}
	if diff := quota - b.Total(); diff > 0 {
		b.credit(common.BalanceBucketGift, diff)
	} else if diff < 0 {
		b.debit(common.DefaultBalanceBucketOrder(), -diff, true)
	}
	return b
}

// BalanceBuckets 返回用户行上的分桶原始值。
func (user *User) BalanceBuckets() BalanceBuckets {
	return BalanceBuckets{
		Topup:       user.QuotaTopup,
		AffRebate:   user.QuotaAffRebate,
		InviteBonus: user.QuotaInviteBonus,
		Gift:        user.QuotaGift,
		Debt:        user.QuotaDebt,
	}
}

// NormalizedBalanceBuckets 返回自愈后的分桶视图（与 quota 保持一致）。
func (user *User) NormalizedBalanceBuckets() BalanceBuckets {
	return user.BalanceBuckets().normalize(user.Quota)
}

// NormalizeBalanceBucketFields 用自愈后的分桶值覆盖用户对象上的桶字段（仅用于展示）。
func (user *User) NormalizeBalanceBucketFields() {
	user.setBalanceBuckets(user.NormalizedBalanceBuckets())
}

func (user *User) setBalanceBuckets(b BalanceBuckets) {
	user.QuotaTopup = b.Topup
	user.QuotaAffRebate = b.AffRebate
	user.QuotaInviteBonus = b.InviteBonus
	user.QuotaGift = b.Gift
	user.QuotaDebt = b.Debt
	user.Quota = b.Total()
}

// setInitialGiftQuota 为新建用户设置注册赠送额度（计入赠送余额）。
func (user *User) setInitialGiftQuota(quota int) {
	user.setBalanceBuckets(BalanceBuckets{Gift: quota})
}

// balanceMutation 在自愈后的快照上修改余额；返回错误则放弃本次写入。
type balanceMutation func(b *BalanceBuckets) error

// mutateUserBalanceTx 在给定事务中读取、修改并以比较并交换方式写回余额。
// 返回 quota 的实际变化量。不负责同步缓存。
func mutateUserBalanceTx(tx *gorm.DB, userId int, fn balanceMutation) (int, error) {
	var row User
	if err := lockForUpdate(tx).Select(append([]string{"id"}, balanceBucketColumns...)).
		Where("id = ?", userId).Take(&row).Error; err != nil {
		return 0, err
	}
	return applyUserBalanceMutationTx(tx, &row, fn)
}

// applyUserBalanceMutationTx 在调用方已加锁读取的用户行上执行余额修改，
// 避免同一事务重复查询用户行。row 必须包含 id、quota 与全部分桶列。
func applyUserBalanceMutationTx(tx *gorm.DB, row *User, fn balanceMutation) (int, error) {
	userId := row.Id
	original := row.BalanceBuckets()
	after := original.normalize(row.Quota)
	if err := fn(&after); err != nil {
		return 0, err
	}
	newQuota := after.Total()
	if after == original && newQuota == row.Quota {
		return 0, nil
	}
	result := tx.Model(&User{}).
		Where("id = ? AND quota = ? AND quota_topup = ? AND quota_aff_rebate = ? AND quota_invite_bonus = ? AND quota_gift = ? AND quota_debt = ?",
			userId, row.Quota, original.Topup, original.AffRebate, original.InviteBonus, original.Gift, original.Debt).
		Updates(map[string]any{
			"quota":              newQuota,
			"quota_topup":        after.Topup,
			"quota_aff_rebate":   after.AffRebate,
			"quota_invite_bonus": after.InviteBonus,
			"quota_gift":         after.Gift,
			"quota_debt":         after.Debt,
		})
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected != 1 {
		return 0, errBalanceConflict
	}
	return newQuota - row.Quota, nil
}

// runBalanceTransaction 执行包含余额修改的事务：比较并交换冲突时整体重试，
// 提交成功后把 fn 返回的 quota 变化量同步到缓存。
func runBalanceTransaction(userId int, fn func(tx *gorm.DB) (int, error)) (int, error) {
	var delta int
	var err error
	for attempt := range maxBalanceUpdateAttempts {
		err = DB.Transaction(func(tx *gorm.DB) error {
			d, txErr := fn(tx)
			delta = d
			return txErr
		})
		if !errors.Is(err, errBalanceConflict) {
			break
		}
		time.Sleep(time.Duration(1+rand.Intn(5*(attempt+1))) * time.Millisecond)
	}
	if err != nil {
		return 0, err
	}
	if delta != 0 {
		if cacheErr := cacheIncrUserQuota(userId, int64(delta)); cacheErr != nil {
			common.SysLog(fmt.Sprintf("failed to sync user %d balance delta to cache: %s", userId, cacheErr.Error()))
		}
	}
	return delta, nil
}

// mutateUserBalance 在独立事务中修改余额，冲突时重试，提交后同步缓存总额。
func mutateUserBalance(userId int, fn balanceMutation) (int, error) {
	return runBalanceTransaction(userId, func(tx *gorm.DB) (int, error) {
		return mutateUserBalanceTx(tx, userId, fn)
	})
}

func creditMutation(bucket string, amount int, limitErr error) balanceMutation {
	return func(b *BalanceBuckets) error {
		if b.Total() > common.MaxWalletQuota-amount {
			return limitErr
		}
		b.credit(bucket, amount)
		return nil
	}
}

func validateCredit(bucket string, amount int) error {
	if bucket != common.BalanceBucketDebt && !common.IsBalanceBucket(bucket) {
		return ErrInvalidBalanceBucket
	}
	if amount < 0 {
		return errors.New("quota 不能为负数！")
	}
	return common.ValidateWalletQuota(amount)
}

// CreditUserBalance 把 amount 计入指定余额桶（独立事务，自动同步缓存）。
func CreditUserBalance(userId int, bucket string, amount int) error {
	if err := validateCredit(bucket, amount); err != nil {
		return err
	}
	if amount == 0 {
		return nil
	}
	_, err := mutateUserBalance(userId, creditMutation(bucket, amount, ErrWalletQuotaLimitExceeded))
	return err
}

// CreditUserBalanceTx 在调用方事务中入账；调用方须在提交后自行同步缓存。
func CreditUserBalanceTx(tx *gorm.DB, userId int, bucket string, amount int, limitErr error) error {
	if err := validateCredit(bucket, amount); err != nil {
		return err
	}
	if amount == 0 {
		return nil
	}
	if limitErr == nil {
		limitErr = ErrWalletQuotaLimitExceeded
	}
	_, err := mutateUserBalanceTx(tx, userId, creditMutation(bucket, amount, limitErr))
	return err
}

// ReserveUserBalance 按分组允许的余额桶顺序严格预扣 amount；
// 可用余额不足时不做任何扣减，返回 *GroupBalanceInsufficientError。
func ReserveUserBalance(userId int, group string, amount int) (common.BalanceLedger, error) {
	if amount < 0 {
		return nil, errors.New("quota 不能为负数！")
	}
	if amount == 0 {
		return nil, nil
	}
	order := setting.GetGroupBalanceBuckets(group)
	var ledger common.BalanceLedger
	_, err := mutateUserBalance(userId, func(b *BalanceBuckets) error {
		available := b.Available(order)
		if available < amount {
			return &GroupBalanceInsufficientError{Group: group, Buckets: order, Available: available, Required: amount}
		}
		ledger, _ = b.debit(order, amount, false)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ledger, nil
}

// DebitUserBalance 结算补扣：按分组顺序扣减，不足部分记为欠费
// （与旧版"余额可为负"的欠费语义一致），总是全额扣减。
func DebitUserBalance(userId int, group string, amount int) (common.BalanceLedger, error) {
	if amount < 0 {
		return nil, errors.New("quota 不能为负数！")
	}
	if amount == 0 {
		return nil, nil
	}
	order := setting.GetGroupBalanceBuckets(group)
	var ledger common.BalanceLedger
	_, err := mutateUserBalance(userId, func(b *BalanceBuckets) error {
		ledger, _ = b.debit(order, amount, true)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ledger, nil
}

// RefundUserBalance 按账本退回原余额桶（一次事务完成）。
func RefundUserBalance(userId int, entries common.BalanceLedger) error {
	total := entries.Total()
	if total <= 0 {
		return nil
	}
	for _, entry := range entries {
		if err := validateCredit(entry.Bucket, entry.Amount); err != nil {
			return err
		}
	}
	_, err := mutateUserBalance(userId, func(b *BalanceBuckets) error {
		if b.Total() > common.MaxWalletQuota-total {
			return ErrWalletQuotaLimitExceeded
		}
		// 先冲回欠费条目，再退回各余额桶：结果与条目顺序无关，
		// 且不会出现"先退的桶额度被拿去还欠费、再冲欠费溢出到赠送"的错位。
		for _, entry := range entries {
			if entry.Bucket == common.BalanceBucketDebt {
				b.credit(entry.Bucket, entry.Amount)
			}
		}
		for _, entry := range entries {
			if entry.Bucket != common.BalanceBucketDebt {
				b.credit(entry.Bucket, entry.Amount)
			}
		}
		return nil
	})
	return err
}

// RefundUserBalanceForGroup 退还来源未知的额度（无扣费账本的旧路径）：
// 计入该分组扣费顺序中的第一个余额桶，即最先被扣的那一类，避免把赠送类
// 余额经退款"洗"成充值余额。
func RefundUserBalanceForGroup(userId int, group string, amount int) error {
	order := setting.GetGroupBalanceBuckets(group)
	return CreditUserBalance(userId, order[0], amount)
}

// RefundUserBalanceForUserGroup 与 RefundUserBalanceForGroup 相同，但在同一事务中
// 从数据库读取用户当前分组（不依赖分组缓存）。
func RefundUserBalanceForUserGroup(userId int, amount int) error {
	if err := validateCredit(common.BalanceBucketTopup, amount); err != nil {
		return err
	}
	if amount == 0 {
		return nil
	}
	_, err := runBalanceTransaction(userId, func(tx *gorm.DB) (int, error) {
		var user User
		if err := tx.Where("id = ?", userId).Take(&user).Error; err != nil { // 读整行：group 在 MySQL/PG 上是保留字
			return 0, err
		}
		order := setting.GetGroupBalanceBuckets(user.Group)
		return mutateUserBalanceTx(tx, userId, creditMutation(order[0], amount, ErrWalletQuotaLimitExceeded))
	})
	return err
}

// RefundUserBalanceWithLedger 退还 amount：优先按账本尾部退回原桶，账本
// 不足的部分按分组规则退还。ledger 会被原地消耗。
func RefundUserBalanceWithLedger(userId int, group string, ledger *common.BalanceLedger, amount int) error {
	if amount <= 0 {
		return nil
	}
	var entries common.BalanceLedger
	remaining := amount
	if ledger != nil {
		entries, remaining = ledger.PopTail(amount)
	}
	if remaining > 0 {
		order := setting.GetGroupBalanceBuckets(group)
		entries = append(entries, common.BalanceLedgerEntry{Bucket: order[0], Amount: remaining})
	}
	return RefundUserBalance(userId, entries)
}

// UserBalanceView 是对外展示的余额明细。
type UserBalanceView struct {
	Quota   int            `json:"quota"`
	Buckets BalanceBuckets `json:"buckets"`
}

// GetUserBalanceBuckets 从数据库读取用户余额明细（自愈视图）。
func GetUserBalanceBuckets(userId int) (*UserBalanceView, error) {
	var row User
	if err := DB.Select(append([]string{"id"}, balanceBucketColumns...)).Where("id = ?", userId).Take(&row).Error; err != nil {
		return nil, err
	}
	return &UserBalanceView{Quota: row.Quota, Buckets: row.NormalizedBalanceBuckets()}, nil
}

// GetUserGroupAvailableBalance 返回某分组当前可用的余额。
func GetUserGroupAvailableBalance(userId int, group string) (int, []string, error) {
	view, err := GetUserBalanceBuckets(userId)
	if err != nil {
		return 0, nil, err
	}
	order := setting.GetGroupBalanceBuckets(group)
	return view.Buckets.Available(order), order, nil
}

// UserBalanceBucketAdjustment 是管理员按桶调整余额的结果快照。
type UserBalanceBucketAdjustment struct {
	UserID   int
	Username string
	Bucket   string
	Before   BalanceBuckets
	After    BalanceBuckets
}

// AdjustUserBalanceBucket 管理员调整单个余额桶：add 增加、subtract 减少（不低于 0）、
// override 设置为指定值。总额随之变化并同步缓存。
func AdjustUserBalanceBucket(userID, operatorRole int, bucket, mode string, value int) (*UserBalanceBucketAdjustment, error) {
	if userID <= 0 || !common.IsBalanceBucket(bucket) {
		return nil, ErrInvalidUserQuotaAdjustment
	}
	if mode != "add" && mode != "subtract" && mode != "override" {
		return nil, ErrInvalidUserQuotaAdjustment
	}
	if value < 0 || (mode != "override" && value == 0) {
		return nil, ErrInvalidUserQuotaAdjustment
	}
	if value > common.MaxWalletQuota {
		return nil, ErrWalletQuotaLimitExceeded
	}
	var adjustment UserBalanceBucketAdjustment
	_, err := runBalanceTransaction(userID, func(tx *gorm.DB) (int, error) {
		var user User
		if err := lockForUpdate(tx).First(&user, userID).Error; err != nil {
			return 0, err
		}
		if operatorRole != common.RoleRootUser && operatorRole <= user.Role {
			return 0, ErrUserQuotaPermission
		}
		adjustment = UserBalanceBucketAdjustment{UserID: user.Id, Username: user.Username, Bucket: bucket}
		return applyUserBalanceMutationTx(tx, &user, func(b *BalanceBuckets) error {
			adjustment.Before = *b
			current := b.Get(bucket)
			switch mode {
			case "add":
				if b.Total() > common.MaxWalletQuota-value {
					return ErrWalletQuotaLimitExceeded
				}
				b.credit(bucket, value)
			case "subtract":
				if value > current {
					return fmt.Errorf("%w: 当前余额 %s", ErrBalanceBucketInsufficient, logger.FormatQuota(current))
				}
				*b.ptr(bucket) -= value
			case "override":
				if b.Total()-current > common.MaxWalletQuota-value {
					return ErrWalletQuotaLimitExceeded
				}
				*b.ptr(bucket) = value
			}
			adjustment.After = *b
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return &adjustment, nil
}

const balanceBucketsMigrationKey = "BalanceBucketsMigrationV1"

// migrateUserBalanceBuckets 一次性把存量用户的 quota 迁入赠送余额（负数迁入欠费）。
// 存量余额来源无法区分，且实际多为注册赠送、邀请奖励等，不能计入权限最高的
// 充值余额；升级后如有用户确属充值，由管理员按桶调整。
// 以 options 表标记防重复；SQL 本身也只处理各桶全为 0 的行，重复执行无副作用。
func migrateUserBalanceBuckets(db *gorm.DB) error {
	var marker Option
	err := db.Where(&Option{Key: balanceBucketsMigrationKey}).Take(&marker).Error
	if err == nil && marker.Value == "done" {
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		emptyBuckets := "quota_topup = 0 AND quota_aff_rebate = 0 AND quota_invite_bonus = 0 AND quota_gift = 0 AND quota_debt = 0"
		positive := tx.Model(&User{}).Unscoped().Where("quota > 0 AND "+emptyBuckets).
			Update("quota_gift", gorm.Expr("quota"))
		if positive.Error != nil {
			return positive.Error
		}
		negative := tx.Model(&User{}).Unscoped().Where("quota < 0 AND "+emptyBuckets).
			Update("quota_debt", gorm.Expr("quota"))
		if negative.Error != nil {
			return negative.Error
		}
		common.SysLog(fmt.Sprintf("balance buckets migration: %d users -> gift, %d users -> debt", positive.RowsAffected, negative.RowsAffected))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&Option{Key: balanceBucketsMigrationKey, Value: "done"}).Error
		}
		return tx.Model(&Option{}).Where(&Option{Key: balanceBucketsMigrationKey}).Update("value", "done").Error
	})
}
