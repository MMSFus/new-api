package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/pkg/sessionrecorder"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SessionRecorderRecord indexes one recorded request. Rows are written by
// the recorder producer (archive columns zero) and completed by the consumer
// or the historical backfill once the batch is verified in R2.
type SessionRecorderRecord struct {
	Id           int    `json:"id"`
	RecordId     string `json:"record_id" gorm:"type:varchar(64);uniqueIndex"`
	SessionId    string `json:"session_id" gorm:"type:varchar(128);index:idx_srr_session_time,priority:1"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;autoCreateTime:false;index:idx_srr_time_model,priority:1;index:idx_srr_session_time,priority:2"`
	DurationMs   int64  `json:"duration_ms" gorm:"bigint"`
	ModelName    string `json:"model_name" gorm:"type:varchar(255);index;index:idx_srr_time_model,priority:2"`
	Group        string `json:"group" gorm:"type:varchar(64)"`
	ChannelId    int    `json:"channel_id"`
	UserId       int    `json:"user_id" gorm:"index"`
	TokenId      int    `json:"token_id"`
	StatusCode   int    `json:"status_code"`
	Stream       bool   `json:"stream"`
	Path         string `json:"path" gorm:"type:varchar(255)"`
	RawBytes     int64  `json:"raw_bytes" gorm:"bigint"`
	ArchiveBytes int64  `json:"archive_bytes" gorm:"bigint"`
	BatchId      string `json:"batch_id" gorm:"type:varchar(64);index"`
	ObjectKey    string `json:"object_key" gorm:"type:varchar(512)"`
	RootId       string `json:"root_id" gorm:"type:varchar(64)"`
	RecordPath   string `json:"record_path" gorm:"type:varchar(512)"`
	ArchivedAt   int64  `json:"archived_at" gorm:"bigint"`
}

const sessionRecorderUpsertBatch = 200

// sessionRecorderProducerColumns are the columns a producer upsert may set.
// Archive columns are excluded so a late producer write never undoes a
// consumer or backfill result.
var sessionRecorderProducerColumns = []string{
	"session_id", "created_at", "duration_ms", "model_name", "group", "channel_id",
	"user_id", "token_id", "status_code", "stream", "path", "raw_bytes", "root_id", "record_path",
}

var sessionRecorderBatchColumns = append(append([]string(nil), sessionRecorderProducerColumns...),
	"archive_bytes", "batch_id", "object_key", "archived_at")

// fitColumn shortens a value to a varchar(n) column on a rune boundary.
// Identifiers (hashed=true) keep a hash suffix so distinct values stay
// distinct.
func fitColumn(value string, n int, hashed bool) string {
	if len(value) <= n {
		return value
	}
	keep := n
	suffix := ""
	if hashed {
		sum := sha256.Sum256([]byte(value))
		suffix = "~" + hex.EncodeToString(sum[:8])
		keep = n - len(suffix)
	}
	for keep > 0 && !utf8.RuneStart(value[keep]) {
		keep--
	}
	return value[:keep] + suffix
}

func sessionRecorderRecordsFromRows(rows []sessionrecorder.IndexRow) []SessionRecorderRecord {
	records := make([]SessionRecorderRecord, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		// A record id that does not fit the column cannot be looked up again.
		if row.RecordID == "" || len(row.RecordID) > 64 {
			continue
		}
		// PostgreSQL rejects one INSERT ... ON CONFLICT touching a row twice.
		if _, dup := seen[row.RecordID]; dup {
			continue
		}
		seen[row.RecordID] = struct{}{}
		records = append(records, SessionRecorderRecord{
			RecordId:     row.RecordID,
			SessionId:    fitColumn(row.SessionID, 128, true),
			CreatedAt:    row.CreatedAt,
			DurationMs:   row.DurationMs,
			ModelName:    fitColumn(row.ModelName, 255, false),
			Group:        fitColumn(row.Group, 64, false),
			ChannelId:    row.ChannelID,
			UserId:       row.UserID,
			TokenId:      row.TokenID,
			StatusCode:   row.StatusCode,
			Stream:       row.Stream,
			Path:         fitColumn(row.Path, 255, false),
			RawBytes:     row.RawBytes,
			ArchiveBytes: row.ArchiveBytes,
			BatchId:      fitColumn(row.BatchID, 64, false),
			ObjectKey:    row.ObjectKey,
			RootId:       row.RootID,
			RecordPath:   row.RecordPath,
			ArchivedAt:   row.ArchivedAt,
		})
	}
	return records
}

func upsertSessionRecorderRecords(ctx context.Context, rows []sessionrecorder.IndexRow, columns []string) error {
	records := sessionRecorderRecordsFromRows(rows)
	if len(records) == 0 {
		return nil
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "record_id"}},
			DoUpdates: clause.AssignmentColumns(columns),
		}).CreateInBatches(&records, sessionRecorderUpsertBatch).Error
	})
}

// UpsertSessionRecorderRecords stores producer rows and keeps archive columns.
func UpsertSessionRecorderRecords(ctx context.Context, rows []sessionrecorder.IndexRow) error {
	return upsertSessionRecorderRecords(ctx, rows, sessionRecorderProducerColumns)
}

// ApplySessionRecorderBatch stores every row of a verified batch in one
// transaction; applying the same batch again changes nothing.
func ApplySessionRecorderBatch(ctx context.Context, rows []sessionrecorder.IndexRow) error {
	return upsertSessionRecorderRecords(ctx, rows, sessionRecorderBatchColumns)
}

// SessionRecorderBatchIndexed reports whether a batch already has archived
// index rows (written by the live consumer or an earlier backfill).
func SessionRecorderBatchIndexed(ctx context.Context, batchID string) (bool, error) {
	var ids []int
	err := DB.WithContext(ctx).Model(&SessionRecorderRecord{}).
		Where("batch_id = ? AND archived_at > 0", batchID).Limit(1).Pluck("id", &ids).Error
	return len(ids) > 0, err
}

// SessionRecorderIndexSink connects the recorder package to this table.
type SessionRecorderIndexSink struct{}

func (SessionRecorderIndexSink) UpsertRecords(ctx context.Context, rows []sessionrecorder.IndexRow) error {
	return UpsertSessionRecorderRecords(ctx, rows)
}

func (SessionRecorderIndexSink) ApplyBatch(ctx context.Context, rows []sessionrecorder.IndexRow) error {
	return ApplySessionRecorderBatch(ctx, rows)
}

// SessionRecorderTerminalSource marks one spool file that a cleaned or
// retained batch already archived, so the consumer scan skips it.
type SessionRecorderTerminalSource struct {
	Id        int    `json:"id"`
	Scope     string `json:"scope" gorm:"type:varchar(32);not null;uniqueIndex:idx_srts_source,priority:1;index:idx_srts_expiry,priority:1;index:idx_srts_batch,priority:1"`
	SourceKey string `json:"source_key" gorm:"type:varchar(32);not null;uniqueIndex:idx_srts_source,priority:2"`
	BatchId   string `json:"batch_id" gorm:"type:varchar(64);not null;uniqueIndex:idx_srts_source,priority:3;index:idx_srts_batch,priority:2"`
	// ExpiresAt is Unix seconds; 0 never expires.
	ExpiresAt int64 `json:"expires_at" gorm:"bigint;not null;index:idx_srts_expiry,priority:2"`
}

const sessionRecorderTerminalSourceBatch = 500

// SessionRecorderSourceLedger stores terminal sources in the main database.
type SessionRecorderSourceLedger struct{}

func (SessionRecorderSourceLedger) AddTerminalBatch(ctx context.Context, batch sessionrecorder.TerminalBatch) error {
	rows := make([]SessionRecorderTerminalSource, 0, len(batch.SourceKeys))
	for _, key := range batch.SourceKeys {
		rows = append(rows, SessionRecorderTerminalSource{Scope: batch.Scope, SourceKey: key, BatchId: batch.BatchID, ExpiresAt: batch.ExpiresAt})
	}
	if len(rows) == 0 {
		return nil
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for chunk := range slices.Chunk(rows, sessionRecorderTerminalSourceBatch) {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (SessionRecorderSourceLedger) ContainsSource(ctx context.Context, scope, sourceKey string, now int64) (bool, error) {
	var ids []int
	err := DB.WithContext(ctx).Model(&SessionRecorderTerminalSource{}).
		Where("scope = ? AND source_key = ? AND (expires_at = 0 OR expires_at > ?)", scope, sourceKey, now).
		Limit(1).Pluck("id", &ids).Error
	return len(ids) > 0, err
}

func (SessionRecorderSourceLedger) ExpiredBatches(ctx context.Context, scope string, now int64, limit int) ([]string, error) {
	var ids []string
	err := DB.WithContext(ctx).Model(&SessionRecorderTerminalSource{}).
		Where("scope = ? AND expires_at > 0 AND expires_at <= ?", scope, now).
		Distinct("batch_id").Order("batch_id").Limit(limit).Pluck("batch_id", &ids).Error
	return ids, err
}

func (SessionRecorderSourceLedger) DeleteBatch(ctx context.Context, scope, batchID string) error {
	return DB.WithContext(ctx).Where("scope = ? AND batch_id = ?", scope, batchID).Delete(&SessionRecorderTerminalSource{}).Error
}

// applySessionRecorderTextFilter matches exactly, or with LIKE when the
// value contains %, like the log search.
func applySessionRecorderTextFilter(tx *gorm.DB, column, value string) (*gorm.DB, error) {
	if value == "" {
		return tx, nil
	}
	if !strings.Contains(value, "%") {
		return tx.Where(column+" = ?", value), nil
	}
	pattern, err := sanitizeLikePattern(value)
	if err != nil {
		return nil, err
	}
	return tx.Where(column+" LIKE ? ESCAPE '!'", pattern), nil
}

type SessionRecorderRevenueFilter struct {
	StartTimestamp int64
	EndTimestamp   int64
	ModelName      string
	BucketSeconds  int64 // 3600 or 86400
}

type SessionRecorderRevenueTotals struct {
	Records            int64   `json:"records"`
	RawBytes           int64   `json:"raw_bytes"`
	ArchiveBytes       int64   `json:"archive_bytes"`
	Revenue            float64 `json:"revenue"`
	UnarchivedRecords  int64   `json:"unarchived_records"`
	UnarchivedRawBytes int64   `json:"unarchived_raw_bytes"`
}

type SessionRecorderRevenueRule struct {
	RuleId       string  `json:"rule_id"`
	RuleName     string  `json:"rule_name"`
	PricePerTB   float64 `json:"price_per_tb"`
	Records      int64   `json:"records"`
	ArchiveBytes int64   `json:"archive_bytes"`
	Revenue      float64 `json:"revenue"`
}

type SessionRecorderRevenueUnmatched struct {
	Records      int64 `json:"records"`
	ArchiveBytes int64 `json:"archive_bytes"`
}

type SessionRecorderRevenueModel struct {
	ModelName    string  `json:"model_name"`
	RuleId       *string `json:"rule_id"`
	Records      int64   `json:"records"`
	RawBytes     int64   `json:"raw_bytes"`
	ArchiveBytes int64   `json:"archive_bytes"`
	Revenue      float64 `json:"revenue"`
}

type SessionRecorderRevenueBucket struct {
	Bucket       int64   `json:"bucket"`
	ArchiveBytes int64   `json:"archive_bytes"`
	Revenue      float64 `json:"revenue"`
}

type SessionRecorderRevenueSummary struct {
	Totals    SessionRecorderRevenueTotals    `json:"totals"`
	ByRule    []SessionRecorderRevenueRule    `json:"by_rule"`
	Unmatched SessionRecorderRevenueUnmatched `json:"unmatched"`
	ByModel   []SessionRecorderRevenueModel   `json:"by_model"`
	Trend     []SessionRecorderRevenueBucket  `json:"trend"`
}

var bytesPerTB = decimal.New(1, 12)

// sessionRecorderRevenue is archive_bytes / 1e12 * price_per_tb.
func sessionRecorderRevenue(archiveBytes int64, pricePerTB float64) decimal.Decimal {
	return decimal.NewFromInt(archiveBytes).Div(bytesPerTB).Mul(decimal.NewFromFloat(pricePerTB))
}

// SumSessionRecorderRevenue aggregates archived records by bucket and model
// in SQL and applies the current rules to each model in Go. Buckets are UTC
// aligned.
func SumSessionRecorderRevenue(filter SessionRecorderRevenueFilter, matcher *sessionrecorder.RevenueMatcher) (*SessionRecorderRevenueSummary, error) {
	if filter.BucketSeconds != 3600 {
		filter.BucketSeconds = 86400
	}
	base := DB.Model(&SessionRecorderRecord{}).Where("created_at >= ? AND created_at <= ?", filter.StartTimestamp, filter.EndTimestamp)
	base, err := applySessionRecorderTextFilter(base, "model_name", filter.ModelName)
	if err != nil {
		return nil, err
	}
	type groupedRow struct {
		Bucket       int64
		ModelName    string
		Records      int64
		RawBytes     int64
		ArchiveBytes int64
	}
	// The bucket width is one of two constants, never user input.
	bucket := "created_at - created_at % 86400"
	if filter.BucketSeconds == 3600 {
		bucket = "created_at - created_at % 3600"
	}
	var grouped []groupedRow
	err = base.Session(&gorm.Session{}).
		Select(bucket + " AS bucket, model_name, COUNT(*) AS records, COALESCE(SUM(raw_bytes), 0) AS raw_bytes, COALESCE(SUM(archive_bytes), 0) AS archive_bytes").
		Where("archive_bytes > 0").
		Group(bucket + ", model_name").
		Scan(&grouped).Error
	if err != nil {
		return nil, err
	}
	var unarchived struct {
		Records  int64
		RawBytes int64
	}
	err = base.Session(&gorm.Session{}).
		Select("COUNT(*) AS records, COALESCE(SUM(raw_bytes), 0) AS raw_bytes").
		Where("archive_bytes = 0").
		Scan(&unarchived).Error
	if err != nil {
		return nil, err
	}

	summary := &SessionRecorderRevenueSummary{ByRule: []SessionRecorderRevenueRule{}, ByModel: []SessionRecorderRevenueModel{}, Trend: []SessionRecorderRevenueBucket{}}
	summary.Totals.UnarchivedRecords, summary.Totals.UnarchivedRawBytes = unarchived.Records, unarchived.RawBytes
	rules := matcher.Rules()
	ruleIndex := make(map[string]int, len(rules))
	ruleRevenue := make([]decimal.Decimal, len(rules))
	for i, rule := range rules {
		ruleIndex[rule.ID] = i
		summary.ByRule = append(summary.ByRule, SessionRecorderRevenueRule{RuleId: rule.ID, RuleName: rule.Name, PricePerTB: rule.PricePerTB})
	}
	type modelTotal struct {
		row     SessionRecorderRevenueModel
		revenue decimal.Decimal
	}
	models := map[string]*modelTotal{}
	buckets := map[int64]*SessionRecorderRevenueBucket{}
	bucketRevenue := map[int64]decimal.Decimal{}
	total := decimal.Zero
	for _, g := range grouped {
		rule, matched := matcher.Match(g.ModelName)
		revenue := decimal.Zero
		if matched {
			revenue = sessionRecorderRevenue(g.ArchiveBytes, rule.PricePerTB)
			i := ruleIndex[rule.ID]
			summary.ByRule[i].Records += g.Records
			summary.ByRule[i].ArchiveBytes += g.ArchiveBytes
			ruleRevenue[i] = ruleRevenue[i].Add(revenue)
		} else {
			summary.Unmatched.Records += g.Records
			summary.Unmatched.ArchiveBytes += g.ArchiveBytes
		}
		m := models[g.ModelName]
		if m == nil {
			m = &modelTotal{row: SessionRecorderRevenueModel{ModelName: g.ModelName}}
			if matched {
				id := rule.ID
				m.row.RuleId = &id
			}
			models[g.ModelName] = m
		}
		m.row.Records += g.Records
		m.row.RawBytes += g.RawBytes
		m.row.ArchiveBytes += g.ArchiveBytes
		m.revenue = m.revenue.Add(revenue)
		b := buckets[g.Bucket]
		if b == nil {
			b = &SessionRecorderRevenueBucket{Bucket: g.Bucket}
			buckets[g.Bucket] = b
		}
		b.ArchiveBytes += g.ArchiveBytes
		bucketRevenue[g.Bucket] = bucketRevenue[g.Bucket].Add(revenue)
		summary.Totals.Records += g.Records
		summary.Totals.RawBytes += g.RawBytes
		summary.Totals.ArchiveBytes += g.ArchiveBytes
		total = total.Add(revenue)
	}
	summary.Totals.Revenue = total.InexactFloat64()
	for i := range summary.ByRule {
		summary.ByRule[i].Revenue = ruleRevenue[i].InexactFloat64()
	}
	for _, m := range models {
		m.row.Revenue = m.revenue.InexactFloat64()
		summary.ByModel = append(summary.ByModel, m.row)
	}
	sort.Slice(summary.ByModel, func(i, j int) bool {
		a, b := summary.ByModel[i], summary.ByModel[j]
		if a.Revenue != b.Revenue {
			return a.Revenue > b.Revenue
		}
		if a.ArchiveBytes != b.ArchiveBytes {
			return a.ArchiveBytes > b.ArchiveBytes
		}
		return a.ModelName < b.ModelName
	})
	for key, b := range buckets {
		b.Revenue = bucketRevenue[key].InexactFloat64()
		summary.Trend = append(summary.Trend, *b)
	}
	sort.Slice(summary.Trend, func(i, j int) bool { return summary.Trend[i].Bucket < summary.Trend[j].Bucket })
	return summary, nil
}

type SessionRecorderSessionFilter struct {
	StartTimestamp int64
	EndTimestamp   int64
	ModelName      string
	UserId         int
	SessionId      string
}

type SessionRecorderSession struct {
	SessionId    string   `json:"session_id"`
	FirstAt      int64    `json:"first_at"`
	LastAt       int64    `json:"last_at"`
	Records      int64    `json:"records"`
	Models       []string `json:"models"`
	UserIds      []int    `json:"user_ids"`
	RawBytes     int64    `json:"raw_bytes"`
	ArchiveBytes int64    `json:"archive_bytes"`
}

func (filter SessionRecorderSessionFilter) query() (*gorm.DB, error) {
	tx := DB.Model(&SessionRecorderRecord{})
	if filter.StartTimestamp > 0 {
		tx = tx.Where("created_at >= ?", filter.StartTimestamp)
	}
	if filter.EndTimestamp > 0 {
		tx = tx.Where("created_at <= ?", filter.EndTimestamp)
	}
	if filter.UserId > 0 {
		tx = tx.Where("user_id = ?", filter.UserId)
	}
	if filter.SessionId != "" {
		tx = tx.Where("session_id = ?", filter.SessionId)
	}
	return applySessionRecorderTextFilter(tx, "model_name", filter.ModelName)
}

// ListSessionRecorderSessions pages sessions by their latest record. The
// filters apply to records, so the aggregates cover the matching records.
func ListSessionRecorderSessions(filter SessionRecorderSessionFilter, offset, limit int) ([]SessionRecorderSession, int64, error) {
	base, err := filter.query()
	if err != nil {
		return nil, 0, err
	}
	var total int64
	grouped := base.Session(&gorm.Session{}).Select("session_id").Group("session_id")
	if err := DB.Table("(?) AS sessions", grouped).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var aggregates []struct {
		SessionId    string
		FirstAt      int64
		LastAt       int64
		Records      int64
		RawBytes     int64
		ArchiveBytes int64
	}
	err = base.Session(&gorm.Session{}).
		Select("session_id, MIN(created_at) AS first_at, MAX(created_at) AS last_at, COUNT(*) AS records, COALESCE(SUM(raw_bytes), 0) AS raw_bytes, COALESCE(SUM(archive_bytes), 0) AS archive_bytes").
		Group("session_id").
		Order("last_at DESC").Order("session_id DESC").
		Offset(max(0, offset)).Limit(limit).
		Scan(&aggregates).Error
	if err != nil {
		return nil, 0, err
	}
	sessions := make([]SessionRecorderSession, 0, len(aggregates))
	if len(aggregates) == 0 {
		return sessions, total, nil
	}
	ids := make([]string, 0, len(aggregates))
	position := make(map[string]int, len(aggregates))
	for i, a := range aggregates {
		sessions = append(sessions, SessionRecorderSession{
			SessionId: a.SessionId, FirstAt: a.FirstAt, LastAt: a.LastAt, Records: a.Records,
			Models: []string{}, UserIds: []int{}, RawBytes: a.RawBytes, ArchiveBytes: a.ArchiveBytes,
		})
		ids = append(ids, a.SessionId)
		position[a.SessionId] = i
	}
	var pairs []struct {
		SessionId string
		ModelName string
		UserId    int
	}
	err = base.Session(&gorm.Session{}).
		Distinct("session_id", "model_name", "user_id").
		Where("session_id IN ?", ids).
		Order("session_id").Order("model_name").Order("user_id").
		Scan(&pairs).Error
	if err != nil {
		return nil, 0, err
	}
	for _, pair := range pairs {
		s := &sessions[position[pair.SessionId]]
		if !slices.Contains(s.Models, pair.ModelName) {
			s.Models = append(s.Models, pair.ModelName)
		}
		if !slices.Contains(s.UserIds, pair.UserId) {
			s.UserIds = append(s.UserIds, pair.UserId)
		}
	}
	for i := range sessions {
		slices.Sort(sessions[i].UserIds)
	}
	return sessions, total, nil
}

// SessionRecorderSessionRecordLimit bounds the records returned for one session.
const SessionRecorderSessionRecordLimit = 1000

// ListSessionRecorderSessionRecords returns a session's records in time order.
func ListSessionRecorderSessionRecords(sessionID string) ([]SessionRecorderRecord, int64, error) {
	tx := DB.Model(&SessionRecorderRecord{}).Where("session_id = ?", sessionID)
	var total int64
	if err := tx.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	records := make([]SessionRecorderRecord, 0)
	err := tx.Order("created_at ASC").Order("id ASC").Limit(SessionRecorderSessionRecordLimit).Find(&records).Error
	return records, total, err
}

// GetSessionRecorderRecord returns nil when the record is not indexed.
func GetSessionRecorderRecord(recordID string) (*SessionRecorderRecord, error) {
	var records []SessionRecorderRecord
	if err := DB.Where("record_id = ?", recordID).Limit(1).Find(&records).Error; err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	return &records[0], nil
}
