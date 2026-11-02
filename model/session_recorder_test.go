package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/pkg/sessionrecorder"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionRecorderIndexAcrossDatabases(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := setupChannelCostDialect(t, dialect)
			// Migrating twice must be a no-op the second time.
			require.NoError(t, db.AutoMigrate(&SessionRecorderRecord{}, &SystemTask{}, &SystemTaskLock{}))
			require.NoError(t, db.AutoMigrate(&SessionRecorderRecord{}, &SystemTask{}, &SystemTaskLock{}))
			t.Cleanup(func() {
				require.NoError(t, db.Migrator().DropTable(&SessionRecorderRecord{}, &SystemTask{}, &SystemTaskLock{}))
			})
			ctx := context.Background()
			const day = int64(1793577600) // 2026-11-02T00:00:00Z
			produced := []sessionrecorder.IndexRow{
				{RecordID: "r1", SessionID: "s1", CreatedAt: day + 100, ModelName: "claude-opus-4", Group: "vip", UserID: 7, RawBytes: 100, RootID: "primary", RecordPath: "m/jsonl/a.jsonl"},
				{RecordID: "r2", SessionID: "s1", CreatedAt: day + 3700, ModelName: "gpt-4o", UserID: 8, RawBytes: 300},
				{RecordID: "r3", SessionID: "s2", CreatedAt: day + 90000, ModelName: "gpt-4o", UserID: 7, RawBytes: 50},
				{RecordID: "r4", SessionID: "s3", CreatedAt: day + 200, ModelName: "unpriced", UserID: 9, RawBytes: 10},
				{RecordID: "r1", SessionID: "dup-in-batch", CreatedAt: day},
			}
			require.NoError(t, UpsertSessionRecorderRecords(ctx, produced))
			require.NoError(t, UpsertSessionRecorderRecords(ctx, produced), "producer upsert is idempotent")
			var count int64
			require.NoError(t, db.Model(&SessionRecorderRecord{}).Count(&count).Error)
			assert.EqualValues(t, 4, count)
			indexed, err := SessionRecorderBatchIndexed(ctx, "b1")
			require.NoError(t, err)
			assert.False(t, indexed)

			batch := []sessionrecorder.IndexRow{produced[0], produced[1], produced[3]}
			for i := range batch {
				batch[i].BatchID, batch[i].ObjectKey, batch[i].ArchivedAt = "b1", "p/b1.tar.gz", day+5000
			}
			sessionrecorder.AllocateArchiveBytes(batch, 1_000_000_000_000) // 1 TB
			require.NoError(t, ApplySessionRecorderBatch(ctx, batch))
			require.NoError(t, ApplySessionRecorderBatch(ctx, batch), "batch apply is idempotent")
			// A late producer write must not undo the archive columns.
			require.NoError(t, UpsertSessionRecorderRecords(ctx, produced[:1]))
			stored, err := GetSessionRecorderRecord("r1")
			require.NoError(t, err)
			require.NotNil(t, stored)
			assert.Equal(t, "b1", stored.BatchId)
			assert.Equal(t, batch[0].ArchiveBytes, stored.ArchiveBytes)
			assert.Equal(t, "vip", stored.Group)
			assert.Equal(t, "m/jsonl/a.jsonl", stored.RecordPath)
			var sum int64
			require.NoError(t, db.Model(&SessionRecorderRecord{}).Where("batch_id = ?", "b1").Select("COALESCE(SUM(archive_bytes), 0)").Scan(&sum).Error)
			assert.EqualValues(t, 1_000_000_000_000, sum)
			indexed, err = SessionRecorderBatchIndexed(ctx, "b1")
			require.NoError(t, err)
			assert.True(t, indexed)
			missing, err := GetSessionRecorderRecord("missing")
			require.NoError(t, err)
			assert.Nil(t, missing)

			matcher, err := sessionrecorder.CompileRevenueRules([]sessionrecorder.RevenueRule{
				{ID: "opus", Name: "Opus", MatchType: sessionrecorder.RevenueMatchGlob, Pattern: "claude-opus-*", PricePerTB: 1200, Enabled: true},
				{ID: "gpt", Name: "GPT", MatchType: sessionrecorder.RevenueMatchKeyword, Pattern: "gpt", PricePerTB: 600, Enabled: true},
			})
			require.NoError(t, err)
			summary, err := SumSessionRecorderRevenue(SessionRecorderRevenueFilter{StartTimestamp: day, EndTimestamp: day + 2*86400, BucketSeconds: 3600}, matcher)
			require.NoError(t, err)
			// r1:r2:r4 raw 100:300:10 of 1e12 bytes.
			opusBytes, gptBytes, otherBytes := batch[0].ArchiveBytes, batch[1].ArchiveBytes, batch[2].ArchiveBytes
			assert.EqualValues(t, 3, summary.Totals.Records)
			assert.EqualValues(t, 410, summary.Totals.RawBytes)
			assert.EqualValues(t, 1_000_000_000_000, summary.Totals.ArchiveBytes)
			assert.EqualValues(t, 1, summary.Totals.UnarchivedRecords)
			assert.EqualValues(t, 50, summary.Totals.UnarchivedRawBytes)
			wantRevenue := float64(opusBytes)/1e12*1200 + float64(gptBytes)/1e12*600
			assert.InDelta(t, wantRevenue, summary.Totals.Revenue, 1e-9)
			require.Len(t, summary.ByRule, 2)
			assert.Equal(t, "opus", summary.ByRule[0].RuleId)
			assert.EqualValues(t, opusBytes, summary.ByRule[0].ArchiveBytes)
			assert.EqualValues(t, gptBytes, summary.ByRule[1].ArchiveBytes)
			assert.EqualValues(t, 1, summary.Unmatched.Records)
			assert.EqualValues(t, otherBytes, summary.Unmatched.ArchiveBytes)
			require.Len(t, summary.ByModel, 3)
			// Ranked by revenue: gpt-4o 300/410 TB at 600 beats opus 100/410 TB at 1200.
			assert.Equal(t, "gpt-4o", summary.ByModel[0].ModelName)
			assert.Equal(t, "claude-opus-4", summary.ByModel[1].ModelName)
			require.NotNil(t, summary.ByModel[0].RuleId)
			assert.Equal(t, "gpt", *summary.ByModel[0].RuleId)
			assert.Equal(t, "unpriced", summary.ByModel[2].ModelName)
			assert.Nil(t, summary.ByModel[2].RuleId)
			require.Len(t, summary.Trend, 2, "two UTC hours have archived data")
			assert.Equal(t, day, summary.Trend[0].Bucket)
			assert.Equal(t, day+3600, summary.Trend[1].Bucket)

			daily, err := SumSessionRecorderRevenue(SessionRecorderRevenueFilter{StartTimestamp: day, EndTimestamp: day + 2*86400, ModelName: "gpt%"}, matcher)
			require.NoError(t, err)
			assert.EqualValues(t, 1, daily.Totals.Records)
			require.Len(t, daily.Trend, 1)
			assert.Equal(t, day, daily.Trend[0].Bucket)
			assert.EqualValues(t, 1, daily.Totals.UnarchivedRecords, "r3 is gpt-4o and not archived")

			sessions, total, err := ListSessionRecorderSessions(SessionRecorderSessionFilter{}, 0, 2)
			require.NoError(t, err)
			assert.EqualValues(t, 3, total)
			require.Len(t, sessions, 2)
			assert.Equal(t, "s2", sessions[0].SessionId, "ordered by latest record")
			assert.Equal(t, "s1", sessions[1].SessionId)
			assert.EqualValues(t, 2, sessions[1].Records)
			assert.Equal(t, day+100, sessions[1].FirstAt)
			assert.Equal(t, day+3700, sessions[1].LastAt)
			assert.ElementsMatch(t, []string{"claude-opus-4", "gpt-4o"}, sessions[1].Models)
			assert.Equal(t, []int{7, 8}, sessions[1].UserIds)
			page2, _, err := ListSessionRecorderSessions(SessionRecorderSessionFilter{}, 2, 2)
			require.NoError(t, err)
			require.Len(t, page2, 1)
			assert.Equal(t, "s3", page2[0].SessionId)
			byUser, total, err := ListSessionRecorderSessions(SessionRecorderSessionFilter{UserId: 8, StartTimestamp: day, EndTimestamp: day + 86400}, 0, 10)
			require.NoError(t, err)
			assert.EqualValues(t, 1, total)
			require.Len(t, byUser, 1)
			assert.EqualValues(t, 1, byUser[0].Records)

			records, total, err := ListSessionRecorderSessionRecords("s1")
			require.NoError(t, err)
			assert.EqualValues(t, 2, total)
			require.Len(t, records, 2)
			assert.Equal(t, "r1", records[0].RecordId)
			assert.Equal(t, "r2", records[1].RecordId)

			// Cancellation marker of the system task framework.
			task, err := CreateSystemTask(SystemTaskTypeSessionRecorderBackfill, struct{}{}, nil)
			require.NoError(t, err)
			requested, err := SystemTaskCancelRequested(task.TaskID)
			require.NoError(t, err)
			assert.False(t, requested)
			asked, err := RequestSystemTaskCancel(SystemTaskTypeSessionRecorderBackfill)
			require.NoError(t, err)
			require.NotNil(t, asked)
			requested, err = SystemTaskCancelRequested(task.TaskID)
			require.NoError(t, err)
			assert.True(t, requested)
		})
	}
}

func TestFitColumnKeepsRuneBoundaryAndIdentity(t *testing.T) {
	long := "会话x"
	for len(long) < 200 {
		long += "会"
	}
	short := fitColumn(long, 128, true)
	assert.LessOrEqual(t, len(short), 128)
	assert.NotEqual(t, short, fitColumn(long+"y", 128, true), "hashed values stay distinct")
	plain := fitColumn(long, 64, false)
	assert.LessOrEqual(t, len(plain), 64)
	assert.Equal(t, "abc", fitColumn("abc", 64, true))
	for _, s := range []string{short, plain} {
		assert.True(t, []rune(s)[0] == '会')
	}
}
