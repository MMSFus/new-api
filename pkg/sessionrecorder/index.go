package sessionrecorder

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/bits"
	"path"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// IndexRow is one recorded request as seen by the session_recorder_records
// index. The package never touches the database: rows go through IndexSink,
// which the host installs with SetIndexSink.
type IndexRow struct {
	RecordID     string
	SessionID    string
	CreatedAt    int64
	DurationMs   int64
	ModelName    string
	Group        string
	ChannelID    int
	UserID       int
	TokenID      int
	StatusCode   int
	Stream       bool
	Path         string
	RawBytes     int64
	ArchiveBytes int64
	BatchID      string
	ObjectKey    string
	RootID       string
	RecordPath   string
	ArchivedAt   int64
}

// IndexSink persists index rows.
//
// UpsertRecords stores rows written by the producer. It must never overwrite
// the archive columns (archive_bytes, batch_id, object_key, archived_at).
//
// ApplyBatch stores every row of one verified batch, archive columns
// included. Applying the same batch again must leave the rows unchanged.
type IndexSink interface {
	UpsertRecords(ctx context.Context, rows []IndexRow) error
	ApplyBatch(ctx context.Context, rows []IndexRow) error
}

type indexSinkHolder struct{ sink IndexSink }

var indexSink atomic.Pointer[indexSinkHolder]

// SetIndexSink installs the index writer; nil disables indexing.
func SetIndexSink(sink IndexSink) {
	if sink == nil {
		indexSink.Store(nil)
		return
	}
	indexSink.Store(&indexSinkHolder{sink: sink})
}

func currentIndexSink() IndexSink {
	if h := indexSink.Load(); h != nil {
		return h.sink
	}
	return nil
}

// AllocateArchiveBytes splits archiveSize over rows in proportion to their
// RawBytes and stores the shares in ArchiveBytes. The shares always sum to
// exactly archiveSize: the remainder of the integer division goes to the
// last row. When no row has raw bytes the size is split evenly by count.
func AllocateArchiveBytes(rows []IndexRow, archiveSize int64) {
	if len(rows) == 0 {
		return
	}
	archiveSize = max(archiveSize, 0)
	var total uint64
	for _, row := range rows {
		total += uint64(max(row.RawBytes, 0))
	}
	var assigned int64
	for i := range rows {
		var share int64
		if total == 0 {
			share = archiveSize / int64(len(rows))
		} else {
			// raw <= total, so the 128-bit quotient fits in 64 bits.
			hi, lo := bits.Mul64(uint64(max(rows[i].RawBytes, 0)), uint64(archiveSize))
			q, _ := bits.Div64(hi, lo, total)
			share = int64(q)
		}
		rows[i].ArchiveBytes = share
		assigned += share
	}
	rows[len(rows)-1].ArchiveBytes += archiveSize - assigned
}

// indexRowFromRecord maps a spool record onto an index row. rawBytes and
// the archive columns are filled in by the caller.
func indexRowFromRecord(rec Record, rootID, recordPath string) IndexRow {
	row := IndexRow{
		RecordID:   rec.RecordID,
		SessionID:  rec.SessionID,
		DurationMs: rec.DurationMs,
		ModelName:  rec.Model,
		Group:      rec.Group,
		ChannelID:  rec.ChannelID,
		UserID:     rec.UserID,
		TokenID:    rec.TokenID,
		StatusCode: rec.StatusCode,
		Stream:     rec.Stream,
		Path:       rec.Path,
		RootID:     rootID,
		RecordPath: recordPath,
	}
	if row.SessionID == "" {
		row.SessionID = row.RecordID
	}
	for _, stamp := range []string{rec.StartedAt, rec.Timestamp} {
		if t, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
			row.CreatedAt = t.Unix()
			break
		}
	}
	return row
}

// maxRecordLineBytes bounds one JSONL line. Lines only carry metadata
// (bodies live in separate files), so a longer line is treated as corrupt.
const maxRecordLineBytes = 8 << 20

// scanRecordLines calls fn with every non-empty line of a record file and
// the number of bytes the line occupies on disk (newline included). A
// ".json" file is a single record.
func scanRecordLines(r io.Reader, name string, fn func(line []byte, size int64) error) error {
	if strings.EqualFold(path.Ext(name), ".json") {
		raw, err := io.ReadAll(io.LimitReader(r, maxRecordLineBytes+1))
		if err != nil {
			return err
		}
		if len(raw) > maxRecordLineBytes {
			return errors.New("record file too large")
		}
		if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 {
			return fn(trimmed, int64(len(raw)))
		}
		return nil
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), maxRecordLineBytes)
	scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			return i + 1, data[:i+1], nil
		}
		if atEOF && len(data) > 0 {
			return len(data), data, nil
		}
		return 0, nil, nil
	})
	for scanner.Scan() {
		raw := scanner.Bytes()
		if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 {
			if err := fn(trimmed, int64(len(raw))); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

// resolveRecordRef maps a body or attachment reference (relative to
// <root>/<model>) to a path relative to the spool root, like
// resolveAttachmentReference does for absolute paths.
func resolveRecordRef(recordPath, ref string) (string, bool) {
	if ref == "" || strings.Contains(ref, "\\") || path.IsAbs(ref) {
		return "", false
	}
	modelDir, _, _ := strings.Cut(recordPath, "/")
	joined := path.Clean(path.Join(modelDir, ref))
	if joined == "." || joined == ".." || strings.HasPrefix(joined, "../") {
		return "", false
	}
	return joined, true
}

// recordFileRefs resolves every body and attachment reference of a record.
func recordFileRefs(rec Record, recordPath string) []string {
	var refs []string
	for _, b := range rec.BodyFiles {
		if rel, ok := resolveRecordRef(recordPath, b.File); ok {
			refs = append(refs, rel)
		}
	}
	for _, a := range rec.Attachments {
		if rel, ok := resolveRecordRef(recordPath, a.File); ok {
			refs = append(refs, rel)
		}
	}
	return refs
}

type pendingIndexRow struct {
	row       IndexRow
	lineBytes int64
	refs      []string
}

// batchIndex collects the records of one batch. Record files are added as
// they are read; sizes of every batch file are registered separately so the
// order in which an archive lists bodies and records does not matter.
type batchIndex struct {
	sizes   map[string]int64 // root_id + "\x00" + relative path
	records []pendingIndexRow
	seen    map[string]struct{}
}

func newBatchIndex() *batchIndex {
	return &batchIndex{sizes: map[string]int64{}, seen: map[string]struct{}{}}
}

func (b *batchIndex) setSize(rootID, rel string, size int64) {
	b.sizes[rootID+"\x00"+rel] = size
}

func (b *batchIndex) addRecordFile(rootID, rel string, r io.Reader) error {
	return scanRecordLines(r, rel, func(line []byte, size int64) error {
		var rec Record
		if err := common.Unmarshal(line, &rec); err != nil {
			return fmt.Errorf("malformed record in %s", rel)
		}
		if rec.RecordID == "" {
			return nil
		}
		if _, dup := b.seen[rec.RecordID]; dup {
			return nil
		}
		b.seen[rec.RecordID] = struct{}{}
		b.records = append(b.records, pendingIndexRow{
			row:       indexRowFromRecord(rec, rootID, rel),
			lineBytes: size,
			refs:      recordFileRefs(rec, rel),
		})
		return nil
	})
}

// rows returns the batch rows with raw and archive bytes assigned.
func (b *batchIndex) rows(m BatchManifest) []IndexRow {
	archivedAt := int64(0)
	if t, err := time.Parse(time.RFC3339Nano, m.CreatedAt); err == nil {
		archivedAt = t.Unix()
	}
	rows := make([]IndexRow, 0, len(b.records))
	for _, p := range b.records {
		row := p.row
		row.RawBytes = p.lineBytes
		for _, ref := range p.refs {
			row.RawBytes += b.sizes[row.RootID+"\x00"+ref]
		}
		row.BatchID, row.ObjectKey, row.ArchivedAt = m.BatchID, m.ObjectKey, archivedAt
		rows = append(rows, row)
	}
	AllocateArchiveBytes(rows, m.ArchiveSize)
	return rows
}

// indexVerifiedBatch writes the index rows of a batch that has just been
// verified in R2, while its sources are still on disk. Failures are logged
// and never block the batch: historical backfill can index it later.
func (c *Consumer) indexVerifiedBatch(ctx context.Context, m *BatchManifest) {
	sink := currentIndexSink()
	if sink == nil {
		return
	}
	index := newBatchIndex()
	for _, e := range m.Sources {
		index.setSize(e.RootID, e.RelativePath, e.Size)
	}
	for _, e := range m.Sources {
		if !isRecordFile(e.RelativePath) {
			continue
		}
		file, _, err := openVerifiedSource(e)
		if err != nil {
			c.logf("session recorder: index batch %s skipped: %s unreadable", m.BatchID, e.RelativePath)
			return
		}
		err = index.addRecordFile(e.RootID, e.RelativePath, file)
		file.Close()
		if err != nil {
			c.logf("session recorder: index batch %s skipped: %v", m.BatchID, err)
			return
		}
	}
	rows := index.rows(*m)
	if len(rows) == 0 {
		return
	}
	writeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := sink.ApplyBatch(writeCtx, rows); err != nil {
		c.logf("session recorder: index batch %s failed: %v", m.BatchID, err)
	}
}
