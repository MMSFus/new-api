package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func useGroupBalanceBuckets(t *testing.T, jsonStr string) {
	t.Helper()
	require.NoError(t, setting.UpdateGroupBalanceBucketsByJSONString(jsonStr))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateGroupBalanceBucketsByJSONString("{}"))
	})
}

func createBalanceTestUser(t *testing.T, group string, buckets BalanceBuckets) User {
	t.Helper()
	user := User{
		Username:    "balance-" + common.GetRandomString(6),
		Password:    "unused-password-hash",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       group,
		AuthVersion: 1,
		AffCode:     "balance-aff-" + common.GetRandomString(8),
	}
	user.setBalanceBuckets(buckets)
	require.NoError(t, DB.Create(&user).Error)
	return user
}

// loadBalance 读取数据库中的余额并校验不变量 quota = 各桶之和 + debt。
func loadBalance(t *testing.T, id int) BalanceBuckets {
	t.Helper()
	var user User
	require.NoError(t, DB.Unscoped().Select(balanceBucketColumns).Where("id = ?", id).Take(&user).Error)
	buckets := user.BalanceBuckets()
	require.Equal(t, user.Quota, buckets.Total(), "quota must equal sum of buckets")
	for _, bucket := range common.DefaultBalanceBucketOrder() {
		require.GreaterOrEqual(t, buckets.Get(bucket), 0, "bucket %s must not be negative", bucket)
	}
	require.LessOrEqual(t, buckets.Debt, 0)
	return buckets
}

func TestBalanceCreditPathsLandInCorrectBucket(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Redemption{}, &Checkin{}))
	truncateTables(t)
	resetBatchUpdateTestState(t)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM redemptions")
		DB.Exec("DELETE FROM checkins")
	})
	useGroupBalanceBuckets(t, `{"vip":["topup","gift"]}`)

	oldNewUser := common.QuotaForNewUser
	common.QuotaForNewUser = 700
	t.Cleanup(func() { common.QuotaForNewUser = oldNewUser })

	tests := []struct {
		name  string
		group string
		start BalanceBuckets
		run   func(t *testing.T, user *User)
		want  BalanceBuckets
	}{
		{
			name:  "redemption code credits topup",
			group: "default",
			run: func(t *testing.T, user *User) {
				key := common.GetRandomString(32)
				require.NoError(t, DB.Create(&Redemption{Key: key, Status: common.RedemptionCodeStatusEnabled, Quota: 300}).Error)
				quota, err := Redeem(key, user.Id)
				require.NoError(t, err)
				assert.Equal(t, 300, quota)
			},
			want: BalanceBuckets{Topup: 300},
		},
		{
			name:  "online top up credits topup",
			group: "default",
			start: BalanceBuckets{Gift: 10},
			run: func(t *testing.T, user *User) {
				require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
					return creditTopUpQuota(tx, user.Id, 250, nil)
				}))
			},
			want: BalanceBuckets{Topup: 250, Gift: 10},
		},
		{
			name:  "check-in with transaction credits gift",
			group: "default",
			run: func(t *testing.T, user *User) {
				_, err := userCheckinWithTransaction(&Checkin{UserId: user.Id, CheckinDate: "2026-01-01", QuotaAwarded: 40}, user.Id, 40)
				require.NoError(t, err)
			},
			want: BalanceBuckets{Gift: 40},
		},
		{
			name:  "check-in without transaction credits gift",
			group: "default",
			run: func(t *testing.T, user *User) {
				_, err := userCheckinWithoutTransaction(&Checkin{UserId: user.Id, CheckinDate: "2026-01-02", QuotaAwarded: 60}, user.Id, 60)
				require.NoError(t, err)
			},
			want: BalanceBuckets{Gift: 60},
		},
		{
			name:  "invitee reward credits invite bonus",
			group: "default",
			run: func(t *testing.T, user *User) {
				require.NoError(t, CreditUserBalance(user.Id, common.BalanceBucketInviteBonus, 90))
			},
			want: BalanceBuckets{InviteBonus: 90},
		},
		{
			name:  "affiliate transfer credits aff rebate",
			group: "default",
			run: func(t *testing.T, user *User) {
				amount := int(common.QuotaPerUnit)
				require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("aff_quota", amount+5).Error)
				require.NoError(t, user.TransferAffQuotaToQuota(amount))
				assert.Equal(t, 5, user.AffQuota)
				assert.Error(t, user.TransferAffQuotaToQuota(amount), "remaining aff quota is insufficient")
			},
			want: BalanceBuckets{AffRebate: int(common.QuotaPerUnit)},
		},
		{
			name:  "admin add credits gift",
			group: "default",
			start: BalanceBuckets{Topup: 5},
			run: func(t *testing.T, user *User) {
				adj, err := AdjustUserQuota(user.Id, common.RoleRootUser, "add", 80)
				require.NoError(t, err)
				assert.Equal(t, 5, adj.Before)
				assert.Equal(t, 85, adj.After)
			},
			want: BalanceBuckets{Topup: 5, Gift: 80},
		},
		{
			name:  "unknown-origin refund credits first bucket of user group",
			group: "vip",
			run: func(t *testing.T, user *User) {
				require.NoError(t, IncreaseUserQuota(user.Id, 33, true))
			},
			want: BalanceBuckets{Topup: 33},
		},
		{
			name:  "credit repays debt first",
			group: "default",
			start: BalanceBuckets{Topup: 7, Debt: -20},
			run: func(t *testing.T, user *User) {
				require.NoError(t, CreditUserBalance(user.Id, common.BalanceBucketGift, 50))
			},
			want: BalanceBuckets{Topup: 7, Gift: 30},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := createBalanceTestUser(t, tt.group, tt.start)
			tt.run(t, &user)
			assert.Equal(t, tt.want, loadBalance(t, user.Id))
		})
	}

	t.Run("new user gift", func(t *testing.T) {
		user := User{Username: "balance-new-" + common.GetRandomString(4), Password: "password123"}
		require.NoError(t, user.Insert(0))
		assert.Equal(t, BalanceBuckets{Gift: 700}, loadBalance(t, user.Id))
	})
}

func TestReserveRespectsGroupBucketsAndRefundsLIFO(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useGroupBalanceBuckets(t, `{"vip":["topup"],"promo":["gift","topup"]}`)

	user := createBalanceTestUser(t, "default", BalanceBuckets{Topup: 100, Gift: 50, InviteBonus: 20})

	available, buckets, err := GetUserGroupAvailableBalance(user.Id, "vip")
	require.NoError(t, err)
	assert.Equal(t, 100, available)
	assert.Equal(t, []string{"topup"}, buckets)

	// 受限分组：可用余额不足时整笔拒绝，不做任何扣减。
	_, err = ReserveUserBalance(user.Id, "vip", 120)
	var groupErr *GroupBalanceInsufficientError
	require.ErrorAs(t, err, &groupErr)
	assert.ErrorIs(t, err, ErrInsufficientGroupBalance)
	assert.Equal(t, 100, groupErr.Available)
	assert.Equal(t, 120, groupErr.Required)
	assert.Equal(t, BalanceBuckets{Topup: 100, Gift: 50, InviteBonus: 20}, loadBalance(t, user.Id))

	reserved, err := TryReserveUserQuota(user.Id, 500)
	require.NoError(t, err)
	assert.False(t, reserved)

	// 受限分组只动允许的桶。
	ledger, err := ReserveUserBalance(user.Id, "vip", 80)
	require.NoError(t, err)
	assert.Equal(t, common.BalanceLedger{{Bucket: "topup", Amount: 80}}, ledger)
	assert.Equal(t, BalanceBuckets{Topup: 20, Gift: 50, InviteBonus: 20}, loadBalance(t, user.Id))
	require.NoError(t, RefundUserBalanceWithLedger(user.Id, "vip", &ledger, 80))
	assert.Empty(t, ledger)
	assert.Equal(t, BalanceBuckets{Topup: 100, Gift: 50, InviteBonus: 20}, loadBalance(t, user.Id))

	// 配置顺序优先：promo 先扣 gift 再扣 topup，跳过未授权的 invite_bonus。
	ledger, err = ReserveUserBalance(user.Id, "promo", 70)
	require.NoError(t, err)
	assert.Equal(t, common.BalanceLedger{{Bucket: "gift", Amount: 50}, {Bucket: "topup", Amount: 20}}, ledger)
	assert.Equal(t, BalanceBuckets{Topup: 80, InviteBonus: 20}, loadBalance(t, user.Id))

	// 部分退款后进先出：先退最后扣的 topup，再退 gift。
	require.NoError(t, RefundUserBalanceWithLedger(user.Id, "promo", &ledger, 30))
	assert.Equal(t, common.BalanceLedger{{Bucket: "gift", Amount: 40}}, ledger)
	assert.Equal(t, BalanceBuckets{Topup: 100, Gift: 10, InviteBonus: 20}, loadBalance(t, user.Id))

	// 未配置分组使用默认顺序 gift → invite_bonus → aff_rebate → topup。
	ledger, err = ReserveUserBalance(user.Id, "default", 40)
	require.NoError(t, err)
	assert.Equal(t, common.BalanceLedger{{Bucket: "gift", Amount: 10}, {Bucket: "invite_bonus", Amount: 20}, {Bucket: "topup", Amount: 10}}, ledger)
	require.NoError(t, RefundUserBalance(user.Id, ledger))
	assert.Equal(t, BalanceBuckets{Topup: 100, Gift: 10, InviteBonus: 20}, loadBalance(t, user.Id))

	// 退款超过账本的部分计入分组首个桶。
	empty := common.BalanceLedger{}
	require.NoError(t, RefundUserBalanceWithLedger(user.Id, "promo", &empty, 5))
	assert.Equal(t, BalanceBuckets{Topup: 100, Gift: 15, InviteBonus: 20}, loadBalance(t, user.Id))
}

func TestDebitOverdraftStaysInsideGroupAndRollsBack(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useGroupBalanceBuckets(t, `{"vip":["topup"]}`)

	user := createBalanceTestUser(t, "default", BalanceBuckets{Topup: 10, Gift: 5})

	// 结算补扣超过分组可用余额：不足部分记为欠费，绝不动用未授权的 gift。
	ledger, err := DebitUserBalance(user.Id, "vip", 30)
	require.NoError(t, err)
	assert.Equal(t, common.BalanceLedger{{Bucket: "topup", Amount: 10}, {Bucket: "debt", Amount: 20}}, ledger)
	assert.Equal(t, BalanceBuckets{Gift: 5, Debt: -20}, loadBalance(t, user.Id))

	// 按账本退款：先冲回欠费，再退回 topup，恢复原状。
	require.NoError(t, RefundUserBalance(user.Id, ledger))
	assert.Equal(t, BalanceBuckets{Topup: 10, Gift: 5}, loadBalance(t, user.Id))

	// 旧的 DecreaseUserQuota 路径按默认顺序扣减并允许欠费。
	require.NoError(t, DecreaseUserQuota(user.Id, 20, false))
	assert.Equal(t, BalanceBuckets{Debt: -5}, loadBalance(t, user.Id))

	// 欠费状态下严格预扣被拒绝，任何入账先还欠费。
	_, err = ReserveUserBalance(user.Id, "", 1)
	assert.ErrorIs(t, err, ErrInsufficientGroupBalance)
	require.NoError(t, CreditUserBalance(user.Id, common.BalanceBucketTopup, 8))
	assert.Equal(t, BalanceBuckets{Topup: 3}, loadBalance(t, user.Id))
}

func TestBalanceHealsDriftAndKeepsCacheInSync(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)

	user := createBalanceTestUser(t, "default", BalanceBuckets{Gift: 10})
	// 旧版本节点只改 quota：差额在下一次写入时自愈进充值余额。
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("quota", 110).Error)
	var drifted User
	require.NoError(t, DB.Where("id = ?", user.Id).Take(&drifted).Error)
	assert.Equal(t, BalanceBuckets{Topup: 100, Gift: 10}, drifted.NormalizedBalanceBuckets())
	require.NoError(t, populateUserCache(drifted))

	ledger, err := ReserveUserBalance(user.Id, "", 15)
	require.NoError(t, err)
	assert.Equal(t, common.BalanceLedger{{Bucket: "gift", Amount: 10}, {Bucket: "topup", Amount: 5}}, ledger)
	assert.Equal(t, BalanceBuckets{Topup: 95}, loadBalance(t, user.Id))

	cached, err := GetUserCache(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 95, cached.Quota, "cache applies the same delta as the database")

	// 负差自愈：quota 被直接调低时按默认顺序扣减。
	require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("quota", 90).Error)
	require.NoError(t, CreditUserBalance(user.Id, common.BalanceBucketGift, 1))
	assert.Equal(t, BalanceBuckets{Topup: 90, Gift: 1}, loadBalance(t, user.Id))
}

func TestAdjustUserBalanceBucket(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)

	user := createBalanceTestUser(t, "default", BalanceBuckets{Topup: 50, Gift: 20})

	tests := []struct {
		name    string
		role    int
		bucket  string
		mode    string
		value   int
		wantErr error
		want    BalanceBuckets
	}{
		{name: "add invite bonus", role: common.RoleRootUser, bucket: "invite_bonus", mode: "add", value: 30, want: BalanceBuckets{Topup: 50, Gift: 20, InviteBonus: 30}},
		{name: "subtract gift", role: common.RoleRootUser, bucket: "gift", mode: "subtract", value: 15, want: BalanceBuckets{Topup: 50, Gift: 5, InviteBonus: 30}},
		{name: "subtract beyond bucket", role: common.RoleRootUser, bucket: "gift", mode: "subtract", value: 6, wantErr: ErrBalanceBucketInsufficient, want: BalanceBuckets{Topup: 50, Gift: 5, InviteBonus: 30}},
		{name: "override topup", role: common.RoleRootUser, bucket: "topup", mode: "override", value: 7, want: BalanceBuckets{Topup: 7, Gift: 5, InviteBonus: 30}},
		{name: "unknown bucket", role: common.RoleRootUser, bucket: "debt", mode: "add", value: 1, wantErr: ErrInvalidUserQuotaAdjustment, want: BalanceBuckets{Topup: 7, Gift: 5, InviteBonus: 30}},
		{name: "same role denied", role: common.RoleCommonUser, bucket: "gift", mode: "add", value: 1, wantErr: ErrUserQuotaPermission, want: BalanceBuckets{Topup: 7, Gift: 5, InviteBonus: 30}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adj, err := AdjustUserBalanceBucket(user.Id, tt.role, tt.bucket, tt.mode, tt.value)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, adj.After)
			}
			assert.Equal(t, tt.want, loadBalance(t, user.Id))
		})
	}
}

func TestMigrateUserBalanceBucketsIsIdempotent(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))
	truncateTables(t)
	resetMarker := func() {
		DB.Where(&Option{Key: balanceBucketsMigrationKey}).Delete(&Option{})
	}
	resetMarker()
	t.Cleanup(resetMarker)

	create := func(quota int, buckets BalanceBuckets) int {
		user := createBalanceTestUser(t, "default", buckets)
		require.NoError(t, DB.Model(&User{}).Where("id = ?", user.Id).Update("quota", quota).Error)
		return user.Id
	}
	positive := create(100, BalanceBuckets{})
	negative := create(-30, BalanceBuckets{})
	zero := create(0, BalanceBuckets{})
	already := create(50, BalanceBuckets{Gift: 50})

	require.NoError(t, migrateUserBalanceBuckets(DB))
	want := map[int]BalanceBuckets{
		positive: {Topup: 100},
		negative: {Debt: -30},
		zero:     {},
		already:  {Gift: 50},
	}
	for id, expected := range want {
		assert.Equal(t, expected, loadBalance(t, id))
	}
	var marker Option
	require.NoError(t, DB.Where(&Option{Key: balanceBucketsMigrationKey}).Take(&marker).Error)
	assert.Equal(t, "done", marker.Value)

	// 标记存在时直接跳过：即便有旧节点新写入的纯 quota 用户也不会重复迁移。
	late := create(40, BalanceBuckets{})
	require.NoError(t, migrateUserBalanceBuckets(DB))
	var lateUser User
	require.NoError(t, DB.Where("id = ?", late).Take(&lateUser).Error)
	assert.Zero(t, lateUser.QuotaTopup)

	// 标记丢失后重跑：已迁移的行不会被重复计入。
	resetMarker()
	require.NoError(t, migrateUserBalanceBuckets(DB))
	for id, expected := range want {
		assert.Equal(t, expected, loadBalance(t, id))
	}
	assert.Equal(t, BalanceBuckets{Topup: 40}, loadBalance(t, late))
}

func TestParseGroupBalanceBuckets(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string][]string
		wantErr bool
	}{
		{name: "empty", input: "", want: map[string][]string{}},
		{name: "valid order", input: `{"vip":["topup","gift"],"*":["gift"]}`, want: map[string][]string{"vip": {"topup", "gift"}, "*": {"gift"}}},
		{name: "invalid json", input: `{"vip":`, wantErr: true},
		{name: "unknown bucket", input: `{"vip":["cash"]}`, wantErr: true},
		{name: "debt not configurable", input: `{"vip":["debt"]}`, wantErr: true},
		{name: "duplicate bucket", input: `{"vip":["gift","gift"]}`, wantErr: true},
		{name: "empty list", input: `{"vip":[]}`, wantErr: true},
		{name: "empty group", input: `{" ":["gift"]}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := setting.ParseGroupBalanceBuckets(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	useGroupBalanceBuckets(t, `{"vip":["topup"],"*":["gift","topup"]}`)
	assert.Equal(t, []string{"topup"}, setting.GetGroupBalanceBuckets("vip"))
	assert.Equal(t, []string{"gift", "topup"}, setting.GetGroupBalanceBuckets("other"))
	require.NoError(t, setting.UpdateGroupBalanceBucketsByJSONString("{}"))
	assert.Equal(t, common.DefaultBalanceBucketOrder(), setting.GetGroupBalanceBuckets("other"))
	assert.Error(t, setting.UpdateGroupBalanceBucketsByJSONString(`{"vip":["nope"]}`))
}
