package model

import (
	"context"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupChannelCostDB(t *testing.T) *gorm.DB {
	t.Helper()
	return setupChannelCostDialect(t, "sqlite")
}

func setupChannelCostDialect(t *testing.T, dialect string) *gorm.DB {
	t.Helper()
	var dialector gorm.Dialector = sqlite.Open(":memory:")
	dbType := common.DatabaseTypeSQLite
	if dialect == "mysql" {
		dsn := os.Getenv("TEST_MYSQL_DSN")
		if dsn == "" {
			t.Skip("TEST_MYSQL_DSN is not configured")
		}
		dialector, dbType = mysql.Open(dsn), common.DatabaseTypeMySQL
	} else if dialect == "postgres" {
		dsn := os.Getenv("TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("TEST_POSTGRES_DSN is not configured")
		}
		dialector, dbType = postgres.Open(dsn), common.DatabaseTypePostgreSQL
	}
	db, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	oldDB, oldLogDB := DB, LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	oldMemoryCache := common.MemoryCacheEnabled
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(dbType, dbType)
	common.MemoryCacheEnabled = false
	initCol()
	t.Cleanup(func() {
		DB, LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		common.MemoryCacheEnabled = oldMemoryCache
		initCol()
	})
	require.NoError(t, db.AutoMigrate(&Log{}, &Channel{}))
	t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&Log{}, &Channel{})) })
	return db
}

func TestChannelCostBackfillAcrossDatabases(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupChannelCostDialect(t, dialect)
			var version string
			query := "SELECT version()"
			if dialect == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("database version: %s", version)
			createCostChannel(t, db, 1, common.GetPointer(0.5))
			createCostChannel(t, db, 2, nil)
			createCostChannel(t, db, 3, common.GetPointer(0.0))
			filter := LogFilter{StartTimestamp: 100, EndTimestamp: 200, ModelName: "model", Username: "user", Group: "vip"}
			logs := []Log{
				{CreatedAt: 100, Type: LogTypeConsume, ChannelId: 1, Quota: 600, Other: `{"group_ratio":2,"user_group_ratio":2,"admin_info":{"simulated_cache":{"original":{"quota":2000}}},"root_info":{"keep":"value"},"large_integer":9007199254740993}`},
				{CreatedAt: 100, Type: LogTypeRefund, ChannelId: 1, Quota: 200, Other: `{"group_ratio":2}`},
				{CreatedAt: 100, Type: LogTypeConsume, ChannelId: 3, Quota: 100, Other: `{"group_ratio":2}`},
				{CreatedAt: 100, Type: LogTypeConsume, ChannelId: 1, Quota: 100, Other: `{"group_ratio":2,"root_info":{"channel_cost":{"cost_quota":17}}}`},
				{CreatedAt: 100, Type: LogTypeConsume, ChannelId: 2, Quota: 100, Other: `{"group_ratio":2}`},
				{CreatedAt: 100, Type: LogTypeConsume, ChannelId: 99, Quota: 100, Other: `{"group_ratio":2}`},
				{CreatedAt: 100, Type: LogTypeConsume, ChannelId: 1, Quota: 100, Other: `{"group_ratio":0}`},
				{CreatedAt: 100, Type: LogTypeConsume, ChannelId: 1, Quota: 100, Other: `{"group_ratio":2,"admin_info":{"simulated_cache":{}}}`},
				{CreatedAt: 100, Type: LogTypeConsume, ChannelId: 1, Quota: 100, Other: `{broken`},
				{CreatedAt: 201, Type: LogTypeConsume, ChannelId: 1, Quota: 100, Other: `{"group_ratio":2}`},
				{CreatedAt: 100, Type: LogTypeError, ChannelId: 1, Quota: 100, Other: `{"group_ratio":2}`},
			}
			for i := range logs {
				logs[i].ModelName, logs[i].Username, logs[i].Group = "model", "user", "vip"
			}
			require.NoError(t, db.Create(&logs).Error)
			ctx := context.Background()
			upperID, total, ratios, err := PrepareChannelCostBackfill(ctx, filter)
			require.NoError(t, err)
			assert.EqualValues(t, 9, total)
			// Later setting edits and inserted historical logs do not change this run.
			require.NoError(t, db.Model(&Channel{}).Where("id = ?", 1).Update("setting", `{"upstream_cost_ratio":0.9}`).Error)
			late := Log{CreatedAt: 100, Type: LogTypeConsume, ChannelId: 1, Quota: 100, ModelName: "model", Username: "user", Group: "vip", Other: `{"group_ratio":2}`}
			require.NoError(t, db.Create(&late).Error)
			// Prove the lock follows the log connection even with a SQLite main DB.
			common.SetMainDatabaseType(common.DatabaseTypeSQLite)
			cursor, delta, err := BackfillChannelCostBatch(ctx, filter, upperID, ChannelCostBackfillCursor{}, ratios, "task-a")
			require.NoError(t, err)
			assert.EqualValues(t, 9, delta.Processed)
			assert.EqualValues(t, 3, delta.Updated)
			assert.EqualValues(t, 5, delta.Skipped)
			assert.EqualValues(t, 1, delta.Reasons["Channel no longer exists"])
			assert.EqualValues(t, 1, delta.Reasons["Channel cost ratio is not configured"])
			assert.EqualValues(t, 3, delta.Reasons["Invalid historical billing data"])
			var stored Log
			require.NoError(t, db.First(&stored, logs[0].Id).Error)
			assert.Equal(t, 600, stored.Quota, "user charge stays intact")
			assert.Equal(t, "9007199254740993", gjson.Get(stored.Other, "large_integer").Raw)
			assert.Equal(t, "value", gjson.Get(stored.Other, "root_info.keep").String())
			assert.Equal(t, 500.0, gjson.Get(stored.Other, "root_info.channel_cost.cost_quota").Float())
			assert.Equal(t, "historical_backfill", gjson.Get(stored.Other, "root_info.channel_cost.source").String())
			_, next, err := BackfillChannelCostBatch(ctx, filter, upperID, cursor, ratios, "task-a")
			require.NoError(t, err)
			assert.Zero(t, next.Processed, "keyset excludes already visited rows")
			_, again, err := BackfillChannelCostBatch(ctx, filter, upperID, ChannelCostBackfillCursor{}, map[int]*float64{1: common.GetPointer(1.0)}, "task-b")
			require.NoError(t, err)
			assert.Zero(t, again.Updated, "rerun preserves existing snapshots")
			require.NoError(t, db.First(&stored, logs[0].Id).Error)
			assert.Equal(t, "task-a", gjson.Get(stored.Other, "root_info.channel_cost.task_id").String())
			common.SetMainDatabaseType(common.DatabaseType(dialect))
			items, _, err := GetChannelCostLogs(filter, 0, 30)
			require.NoError(t, err)
			var estimated *ChannelCostSnapshot
			for _, item := range items {
				if item.Id == logs[0].Id {
					estimated = item.Cost
				}
			}
			require.NotNil(t, estimated)
			assert.Equal(t, "historical_backfill", estimated.Source)
			assert.Equal(t, "task-a", estimated.TaskID)
			// Cross a batch boundary at one timestamp, with every selected
			// dimension excluding otherwise eligible records.
			pageFilter := LogFilter{StartTimestamp: 300, EndTimestamp: 300, Channel: 1, ModelName: "paging", Username: "selected", Group: "vip"}
			pageLogs := make([]Log, ChannelCostBackfillBatchSize+1)
			for i := range pageLogs {
				pageLogs[i] = Log{CreatedAt: 300, Type: LogTypeConsume, ChannelId: 1, ModelName: "paging", Username: "selected", Group: "vip", Quota: 100, Other: `{"group_ratio":2}`}
			}
			excluded := []Log{
				{CreatedAt: 300, Type: LogTypeConsume, ChannelId: 3, ModelName: "paging", Username: "selected", Group: "vip", Other: `{"group_ratio":2}`},
				{CreatedAt: 300, Type: LogTypeConsume, ChannelId: 1, ModelName: "other", Username: "selected", Group: "vip", Other: `{"group_ratio":2}`},
				{CreatedAt: 300, Type: LogTypeConsume, ChannelId: 1, ModelName: "paging", Username: "other", Group: "vip", Other: `{"group_ratio":2}`},
				{CreatedAt: 300, Type: LogTypeConsume, ChannelId: 1, ModelName: "paging", Username: "selected", Group: "other", Other: `{"group_ratio":2}`},
			}
			require.NoError(t, db.Create(&pageLogs).Error)
			require.NoError(t, db.Create(&excluded).Error)
			bound, count, pageRatios, err := PrepareChannelCostBackfill(ctx, pageFilter)
			require.NoError(t, err)
			assert.EqualValues(t, 101, count)
			pageCursor, firstPage, err := BackfillChannelCostBatch(ctx, pageFilter, bound, ChannelCostBackfillCursor{}, pageRatios, "pages")
			require.NoError(t, err)
			assert.EqualValues(t, 100, firstPage.Updated)
			_, secondPage, err := BackfillChannelCostBatch(ctx, pageFilter, bound, pageCursor, pageRatios, "pages")
			require.NoError(t, err)
			assert.EqualValues(t, 1, secondPage.Updated)
			for _, row := range excluded {
				var untouched Log
				require.NoError(t, db.First(&untouched, row.Id).Error)
				assert.False(t, gjson.Get(untouched.Other, "root_info.channel_cost").Exists())
			}
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel()
			_, _, err = BackfillChannelCostBatch(cancelCtx, filter, upperID, ChannelCostBackfillCursor{}, ratios, "cancelled")
			require.Error(t, err)
		})
	}
}

func TestHistoricalChannelCostRejectsUnreliableMetadata(t *testing.T) {
	for _, other := range []string{`{}`, `{"group_ratio":"2"}`, `{"group_ratio":1e-320}`, `{"group_ratio":2,"root_info":[]}`, `{"group_ratio":2,"root_info":{"channel_cost":{}}}`, `{"group_ratio":2,"admin_info":{"simulated_cache":{"original":{"quota":1.5}}}}`} {
		t.Run(other, func(t *testing.T) {
			result, reason := HistoricalChannelCost(Log{ChannelId: 1, Quota: 100, Other: other}, map[int]*float64{1: common.GetPointer(0.5)}, "task", 100)
			assert.Empty(t, result)
			assert.Equal(t, "Invalid historical billing data", reason)
		})
	}
}

func createCostChannel(t *testing.T, db *gorm.DB, id int, ratio *float64) {
	t.Helper()
	channel := &Channel{Id: id, Name: "channel", Key: "k"}
	channel.SetSetting(dto.ChannelSettings{UpstreamCostRatio: ratio})
	require.NoError(t, db.Create(channel).Error)
}

func TestAttachChannelCostSnapshotsUpstreamCost(t *testing.T) {
	db := setupChannelCostDB(t)
	createCostChannel(t, db, 1, common.GetPointer(0.3))
	createCostChannel(t, db, 2, nil)
	createCostChannel(t, db, 3, common.GetPointer(0.0))

	tests := []struct {
		name       string
		channelId  int
		quota      int
		groupRatio float64
		basis      *int
		want       *ChannelCostSnapshot
	}{
		{
			name: "official price times upstream ratio", channelId: 1, quota: 1500, groupRatio: 1.5,
			want: &ChannelCostSnapshot{UpstreamRatio: 0.3, GroupRatio: 1.5, BaseQuota: 1000, CostQuota: 300},
		},
		{
			name: "simulated cache costs the upstream usage", channelId: 1, quota: 600, groupRatio: 2, basis: common.GetPointer(2000),
			want: &ChannelCostSnapshot{UpstreamRatio: 0.3, GroupRatio: 2, BaseQuota: 1000, CostQuota: 300},
		},
		{
			name: "free channel costs nothing", channelId: 3, quota: 1000, groupRatio: 1,
			want: &ChannelCostSnapshot{UpstreamRatio: 0, GroupRatio: 1, BaseQuota: 1000, CostQuota: 0},
		},
		{name: "unset ratio leaves the log uncosted", channelId: 2, quota: 1000, groupRatio: 1},
		{name: "free group leaves the log uncosted", channelId: 1, quota: 0, groupRatio: 0},
		{name: "missing channel leaves the log uncosted", channelId: 99, quota: 1000, groupRatio: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			other := NewLogOther()
			if tt.basis != nil {
				other.SetCostBasisQuota(*tt.basis)
			}
			attachChannelCost(other, tt.channelId, tt.quota, tt.groupRatio)
			stored := other.JSONString()
			snapshot := gjson.Get(stored, "root_info.channel_cost")
			if tt.want == nil {
				assert.False(t, snapshot.Exists(), stored)
				return
			}
			var got ChannelCostSnapshot
			require.NoError(t, common.UnmarshalJsonStr(snapshot.Raw, &got))
			assert.InDelta(t, tt.want.CostQuota, got.CostQuota, 1e-9)
			assert.InDelta(t, tt.want.BaseQuota, got.BaseQuota, 1e-9)
			assert.Equal(t, tt.want.UpstreamRatio, got.UpstreamRatio)
			assert.Equal(t, tt.want.GroupRatio, got.GroupRatio)
			assert.NotContains(t, stored, "cost_basis", "the basis override is never stored")
		})
	}
}

func TestSumChannelCostNetsRefundsAndSplitsUncostedRevenue(t *testing.T) {
	db := setupChannelCostDB(t)
	createCostChannel(t, db, 1, common.GetPointer(0.5))

	cost := func(costQuota float64) string {
		return common.MapToJsonStr(map[string]any{"root_info": map[string]any{
			"channel_cost": map[string]any{"upstream_ratio": 0.5, "group_ratio": 1, "base_quota": costQuota * 2, "cost_quota": costQuota},
		}})
	}
	now := common.GetTimestamp()
	hour := now - now%3600
	rows := []*Log{
		{Type: LogTypeConsume, CreatedAt: hour + 1, UserId: 7, Username: "u7", ChannelId: 1, ModelName: "a", Group: "vip",
			Quota: 1000, PromptTokens: 10, CompletionTokens: 5, Other: cost(400)},
		{Type: LogTypeConsume, CreatedAt: hour + 2, UserId: 8, Username: "u8", ChannelId: 1, ModelName: "b", Group: "default",
			Quota: 3000, Other: common.MapToJsonStr(map[string]any{
				"root_info":  map[string]any{"channel_cost": map[string]any{"cost_quota": 900}},
				"admin_info": map[string]any{"simulated_cache": map[string]any{"quota_delta": -200}},
			})},
		// A refund takes back revenue and cost.
		{Type: LogTypeRefund, CreatedAt: hour + 3, UserId: 7, Username: "u7", ChannelId: 1, ModelName: "a", Group: "vip",
			Quota: 500, Other: cost(200)},
		// A log written before the channel had a ratio has no snapshot.
		{Type: LogTypeConsume, CreatedAt: hour + 4, UserId: 7, Username: "u7", ChannelId: 1, ModelName: "a", Group: "vip",
			Quota: 700, Other: `{}`},
		// Other log types and logs outside the range never count.
		{Type: LogTypeError, CreatedAt: hour + 5, UserId: 7, ChannelId: 1, Quota: 99, Other: cost(99)},
		{Type: LogTypeConsume, CreatedAt: hour - 40*24*3600, UserId: 7, ChannelId: 1, Quota: 99, Other: cost(99)},
	}
	require.NoError(t, db.Create(&rows).Error)

	report, err := SumChannelCost(LogFilter{StartTimestamp: hour, EndTimestamp: hour + 3600})
	require.NoError(t, err)

	assert.Equal(t, int64(3), report.Total.Requests)
	assert.Equal(t, int64(1), report.Total.Refunds)
	assert.Equal(t, int64(1), report.Total.UncostedRequests)
	assert.Equal(t, int64(4200), report.Total.Revenue)
	assert.Equal(t, int64(3500), report.Total.CostedRevenue)
	assert.InDelta(t, 1100, report.Total.Cost, 1e-9)
	assert.InDelta(t, 2400, report.Total.Profit, 1e-9)
	assert.Equal(t, int64(-200), report.Total.SimulatedCacheDelta)
	assert.Equal(t, int64(10), report.Total.PromptTokens)

	require.Len(t, report.Trend, 1)
	assert.Equal(t, hour, report.Trend[0].Time)

	require.Len(t, report.Channels, 1)
	assert.Equal(t, "channel", report.Channels[0].ChannelName)
	require.NotNil(t, report.Channels[0].UpstreamRatio)
	assert.Equal(t, 0.5, *report.Channels[0].UpstreamRatio)

	require.Len(t, report.Models, 2)
	assert.Equal(t, "b", report.Models[0].ModelName, "ranked by revenue")
	assert.Equal(t, int64(1200), report.Models[1].Revenue)
	assert.Equal(t, 2, report.UserCount)
	assert.Equal(t, 8, report.Users[0].UserId)
	assert.Len(t, report.Groups, 2)

	logs, total, err := GetChannelCostLogs(LogFilter{StartTimestamp: hour, EndTimestamp: hour + 3600, Channel: 1}, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 4, total)
	require.Len(t, logs, 4)
	assert.Nil(t, logs[0].Cost, "newest log has no snapshot")
	require.NotNil(t, logs[1].Cost)
	assert.InDelta(t, 200, logs[1].Cost.CostQuota, 1e-9)
	require.NotNil(t, logs[2].SimulatedCacheDelta)
	assert.Equal(t, int64(-200), *logs[2].SimulatedCacheDelta)
}

func TestHideSimulatedCacheAuditKeepsOtherAdminInfo(t *testing.T) {
	log := &Log{Other: `{"admin_info":{"simulated_cache":{"quota_delta":1},"use_channel":[1]},"cache_tokens":3}`}
	log.HideSimulatedCacheAudit()
	assert.False(t, gjson.Get(log.Other, "admin_info.simulated_cache").Exists())
	assert.True(t, gjson.Get(log.Other, "admin_info.use_channel").Exists())
	assert.Equal(t, int64(3), gjson.Get(log.Other, "cache_tokens").Int())
}
