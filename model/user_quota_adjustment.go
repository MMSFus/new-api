package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var (
	ErrInvalidUserQuotaAdjustment = errors.New("invalid user quota adjustment")
	ErrUserQuotaPermission        = errors.New("cannot adjust quota for this user role")
)

// UserQuotaAdjustment is the immutable database snapshot of a committed manual
// adjustment. Pending relay deductions in the quota cache are not part of it.
type UserQuotaAdjustment struct {
	UserID   int
	Username string
	Before   int
	After    int
}

func AdjustUserQuota(userID, operatorRole int, mode string, value int) (*UserQuotaAdjustment, error) {
	if userID <= 0 || (mode != "add" && mode != "subtract" && mode != "override") {
		return nil, ErrInvalidUserQuotaAdjustment
	}
	if mode != "override" && value <= 0 {
		return nil, ErrInvalidUserQuotaAdjustment
	}
	if value > common.MaxWalletQuota || value < -common.MaxWalletQuota {
		return nil, ErrWalletQuotaLimitExceeded
	}

	// 管理员调整总额：增加部分计入赠送余额，减少部分按默认顺序扣减（不足记欠费）。
	// 按桶调整请使用 AdjustUserBalanceBucket。
	var adjustment UserQuotaAdjustment
	_, err := runBalanceTransaction(userID, func(tx *gorm.DB) (int, error) {
		var user User
		if err := lockForUpdate(tx).First(&user, userID).Error; err != nil {
			return 0, err
		}
		if operatorRole != common.RoleRootUser && operatorRole <= user.Role {
			return 0, ErrUserQuotaPermission
		}
		return applyUserBalanceMutationTx(tx, &user, func(b *BalanceBuckets) error {
			before := b.Total()
			if before > common.MaxWalletQuota || before < -common.MaxWalletQuota {
				return ErrWalletQuotaLimitExceeded
			}
			quota := decimal.NewFromInt(int64(value))
			switch mode {
			case "add":
				quota = decimal.NewFromInt(int64(before)).Add(quota)
			case "subtract":
				quota = decimal.NewFromInt(int64(before)).Sub(quota)
			}
			after, err := common.WalletQuotaFromDecimalStrict(quota)
			if err != nil {
				return ErrWalletQuotaLimitExceeded
			}
			if after > before {
				b.credit(common.BalanceBucketGift, after-before)
			} else if after < before {
				b.debit(common.DefaultBalanceBucketOrder(), before-after, true)
			}
			adjustment = UserQuotaAdjustment{UserID: user.Id, Username: user.Username, Before: before, After: after}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return &adjustment, nil
}
