package sessionrecorder

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// RecordSchemaVersion is the version of the JSONL record written below.
const RecordSchemaVersion = 1

// BodyFile references a request or response body stored next to the record.
type BodyFile struct {
	Kind        string `json:"kind"`
	File        string `json:"file"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	ContentType string `json:"content_type,omitempty"`
	Truncated   bool   `json:"truncated"`
	OriginalLen int64  `json:"original_size"`
}

// Attachment is reserved for media files (none are produced by new-api yet).
type Attachment struct {
	File        string `json:"file"`
	ContentType string `json:"content_type,omitempty"`
	Size        int64  `json:"size"`
}

// Record is one line of the spool JSONL. It never contains credentials:
// Authorization, API keys and cookies are stripped before it is built.
type Record struct {
	SchemaVersion  int               `json:"schema_version"`
	RecordID       string            `json:"record_id"`
	SessionID      string            `json:"session_id"`
	Timestamp      string            `json:"timestamp"`
	StartedAt      string            `json:"started_at"`
	DurationMs     int64             `json:"duration_ms"`
	SiteID         string            `json:"site_id"`
	HostID         string            `json:"host_id"`
	Method         string            `json:"method"`
	Path           string            `json:"path"`
	Stream         bool              `json:"stream"`
	StatusCode     int               `json:"status_code"`
	ChannelID      int               `json:"channel_id"`
	ChannelName    string            `json:"channel_name,omitempty"`
	ChannelType    int               `json:"channel_type"`
	Group          string            `json:"group"`
	Model          string            `json:"model"`
	UpstreamModel  string            `json:"upstream_model,omitempty"`
	UserID         int               `json:"user_id"`
	TokenID        int               `json:"token_id"`
	RequestHeaders map[string]string `json:"request_headers,omitempty"`
	Attachments    []Attachment      `json:"attachments"`
	BodyFiles      []BodyFile        `json:"body_files"`
}

// Capture is one finished request handed to the producer. The byte slices
// are owned by the producer after Enqueue.
type Capture struct {
	Record          Record
	RequestBody     []byte
	RequestFull     int64
	RequestType     string
	ResponseBody    []byte
	ResponseFull    int64
	ResponseType    string
	RequestCapped   bool
	ResponseCapped  bool
	FinishedAt      time.Time
	reservedBytes   int64
	IncludeRequest  bool
	IncludeResponse bool
}

// ProducerCounters are exposed through the admin status API.
type ProducerCounters struct {
	Enqueued       int64  `json:"enqueued"`
	Written        int64  `json:"written"`
	DroppedFull    int64  `json:"dropped_queue_full"`
	DroppedBudget  int64  `json:"dropped_memory_budget"`
	DroppedStorage int64  `json:"dropped_storage"`
	WriteErrors    int64  `json:"write_errors"`
	QueueLength    int    `json:"queue_length"`
	QueueCapacity  int    `json:"queue_capacity"`
	InflightBytes  int64  `json:"inflight_bytes"`
	UsagePercent   int    `json:"usage_percent"`
	StoragePaused  bool   `json:"storage_paused"`
	LastError      string `json:"last_error,omitempty"`
}

// Producer writes captures asynchronously. Enqueue never blocks: when the
// queue or the memory budget is exhausted the capture is dropped and
// counted, so recording can never slow down or fail a relay request.
type Producer struct {
	settings Settings
	root     string
	instance string
	queue    chan *Capture
	maxBytes int64

	inflight       atomic.Int64
	enqueued       atomic.Int64
	written        atomic.Int64
	droppedFull    atomic.Int64
	droppedBudget  atomic.Int64
	droppedStorage atomic.Int64
	writeErrors    atomic.Int64
	storagePaused  atomic.Bool
	lastErr        atomic.Pointer[string]

	stop chan struct{}
	done chan struct{}
	once sync.Once
}

// DefaultMaxQueueBytes bounds memory held by captures in flight.
const DefaultMaxQueueBytes = 256 << 20

func NewProducer(s Settings) *Producer {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return &Producer{
		settings: s,
		root:     filepath.Clean(s.SpoolRoot),
		instance: hex.EncodeToString(b[:]),
		queue:    make(chan *Capture, max(1, s.QueueSize)),
		maxBytes: DefaultMaxQueueBytes,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Settings returns the settings the producer was built with.
func (p *Producer) Settings() Settings { return p.settings }

// Reserve claims n bytes of the capture memory budget.
func (p *Producer) Reserve(n int64) bool {
	if n <= 0 {
		return true
	}
	if p.storagePaused.Load() {
		return false
	}
	if p.inflight.Add(n) > p.maxBytes {
		p.inflight.Add(-n)
		return false
	}
	return true
}

// Release returns bytes previously claimed with Reserve.
func (p *Producer) Release(n int64) {
	if n > 0 {
		p.inflight.Add(-n)
	}
}

// Accepting reports whether new captures would currently be accepted.
func (p *Producer) Accepting() bool {
	return !p.storagePaused.Load() && len(p.queue) < cap(p.queue)
}

// NoteBudgetDrop counts a capture skipped because of the memory budget.
func (p *Producer) NoteBudgetDrop() { p.droppedBudget.Add(1) }

// Enqueue hands a capture to the writer. reserved is the number of bytes
// the caller reserved for it; they are released once written or dropped.
func (p *Producer) Enqueue(c *Capture, reserved int64) bool {
	c.reservedBytes = reserved
	if p.storagePaused.Load() {
		p.droppedStorage.Add(1)
		p.Release(reserved)
		return false
	}
	select {
	case p.queue <- c:
		p.enqueued.Add(1)
		return true
	default:
		p.droppedFull.Add(1)
		p.Release(reserved)
		return false
	}
}

// UsagePercent is max(queue length, memory budget) usage.
func (p *Producer) UsagePercent() int {
	q := len(p.queue) * 100 / max(1, cap(p.queue))
	m := int(p.inflight.Load() * 100 / max(1, p.maxBytes))
	return min(100, max(q, m))
}

// Stats returns the producer stats as written to the stats file.
func (p *Producer) Stats() ProducerStats {
	return ProducerStats{Timestamp: time.Now().UTC().Format(time.RFC3339Nano), ExportQueueUsagePercent: p.UsagePercent()}
}

func (p *Producer) Counters() ProducerCounters {
	c := ProducerCounters{
		Enqueued:       p.enqueued.Load(),
		Written:        p.written.Load(),
		DroppedFull:    p.droppedFull.Load(),
		DroppedBudget:  p.droppedBudget.Load(),
		DroppedStorage: p.droppedStorage.Load(),
		WriteErrors:    p.writeErrors.Load(),
		QueueLength:    len(p.queue),
		QueueCapacity:  cap(p.queue),
		InflightBytes:  p.inflight.Load(),
		UsagePercent:   p.UsagePercent(),
		StoragePaused:  p.storagePaused.Load(),
	}
	if e := p.lastErr.Load(); e != nil {
		c.LastError = *e
	}
	return c
}

func (p *Producer) setErr(err error) {
	msg := err.Error()
	p.lastErr.Store(&msg)
	p.writeErrors.Add(1)
}

// Start runs the writer goroutine. Panics are recovered and the writer
// restarts after a short pause.
func (p *Producer) Start() {
	go func() {
		defer close(p.done)
		for {
			if p.runSafe() {
				return
			}
			select {
			case <-p.stop:
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
}

func (p *Producer) runSafe() (stopped bool) {
	defer func() {
		if r := recover(); r != nil {
			p.setErr(errors.New("producer panic recovered"))
			common.SysError("session recorder producer panic recovered")
			stopped = false
		}
	}()
	p.loop()
	return true
}

// Stop drains what is queued (bounded by timeout) and stops the writer.
func (p *Producer) Stop(timeout time.Duration) {
	p.once.Do(func() { close(p.stop) })
	select {
	case <-p.done:
	case <-time.After(timeout):
	}
}

func (p *Producer) loop() {
	statsTicker := time.NewTicker(5 * time.Second)
	defer statsTicker.Stop()
	p.checkStorage()
	p.writeStats()
	lastStorage := time.Now()
	for {
		select {
		case <-p.stop:
			for {
				select {
				case c := <-p.queue:
					p.write(c)
				default:
					p.writeStats()
					return
				}
			}
		case c := <-p.queue:
			p.write(c)
		case <-statsTicker.C:
			p.writeStats()
		}
		if time.Since(lastStorage) > 10*time.Second {
			p.checkStorage()
			lastStorage = time.Now()
		}
	}
}

func (p *Producer) checkStorage() {
	target := p.root
	if _, err := os.Stat(target); err != nil {
		target = filepath.Dir(target)
	}
	p.storagePaused.Store(storageFull(target))
}

func (p *Producer) writeStats() {
	statsPath := p.settings.EffectiveStatsPath()
	if statsPath == "" {
		return
	}
	raw, err := common.Marshal(p.Stats())
	if err != nil {
		return
	}
	if err := atomicWrite(statsPath, raw, 0o644); err != nil {
		msg := err.Error()
		p.lastErr.Store(&msg)
	}
}

var unsafeSegment = strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_", "\x00", "_")

// SafeSegment turns an arbitrary value into one safe path segment.
func SafeSegment(v, fallback string) string {
	v = strings.TrimSpace(unsafeSegment.Replace(v))
	var b strings.Builder
	for _, r := range v {
		if r < 0x20 || r == 0x7f {
			b.WriteByte('_')
			continue
		}
		b.WriteRune(r)
	}
	v = b.String()
	if v == "" || v == "." || v == ".." || strings.HasPrefix(v, ".") {
		v = fallback + v
	}
	if len(v) > 120 {
		sum := sha256.Sum256([]byte(v))
		v = v[:80] + "-" + hex.EncodeToString(sum[:8])
	}
	return v
}

// spoolLayout returns the model-relative directories for a capture.
func spoolLayout(model, session string, t time.Time) (modelDir, jsonlRel, bodyDir string) {
	modelDir = SafeSegment(model, "unknown")
	stamp := t.UTC().Format("2006/01/02/15")
	sum := sha256.Sum256([]byte(session))
	shard := hex.EncodeToString(sum[:2])
	jsonlRel = path.Join("jsonl", stamp)
	bodyDir = path.Join("body", stamp, shard[:2], shard[2:4], SafeSegment(session, "s"))
	return
}

func writeFileRetry(name string, data []byte, flag int, perm os.FileMode) error {
	for attempt := range 2 {
		if err := os.MkdirAll(filepath.Dir(name), 0o750); err != nil {
			return err
		}
		f, err := os.OpenFile(name, flag, perm)
		if errors.Is(err, fs.ErrNotExist) && attempt == 0 {
			continue // a concurrent cleanup removed the empty directory
		}
		if err != nil {
			return err
		}
		_, werr := f.Write(data)
		cerr := f.Close()
		if werr != nil {
			return werr
		}
		return cerr
	}
	return fs.ErrNotExist
}

func (p *Producer) write(c *Capture) {
	defer p.Release(c.reservedBytes)
	defer func() {
		if r := recover(); r != nil {
			p.setErr(errors.New("producer write panic recovered"))
		}
	}()
	if p.storagePaused.Load() {
		p.droppedStorage.Add(1)
		return
	}
	t := c.FinishedAt
	if t.IsZero() {
		t = time.Now()
	}
	rec := c.Record
	modelDir, jsonlRel, bodyDir := spoolLayout(rec.Model, rec.SessionID, t)
	base := filepath.Join(p.root, modelDir)
	prefix := SafeSegment(t.UTC().Format("20060102T150405.000000000Z")+"_"+rec.RecordID, "r")
	rec.BodyFiles = rec.BodyFiles[:0]
	if rec.Attachments == nil {
		rec.Attachments = []Attachment{}
	}
	bodies := []struct {
		kind, ctype string
		data        []byte
		full        int64
		capped      bool
		include     bool
	}{
		{"request", c.RequestType, c.RequestBody, c.RequestFull, c.RequestCapped, c.IncludeRequest},
		{"response", c.ResponseType, c.ResponseBody, c.ResponseFull, c.ResponseCapped, c.IncludeResponse},
	}
	for _, b := range bodies {
		if !b.include {
			continue
		}
		ext := ".body"
		if b.kind == "response" && rec.Stream {
			ext = ".sse"
		}
		rel := path.Join(bodyDir, prefix+"_"+b.kind+ext)
		sum := sha256.Sum256(b.data)
		if err := writeFileRetry(filepath.Join(base, filepath.FromSlash(rel)), b.data, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640); err != nil {
			p.setErr(err)
			return
		}
		rec.BodyFiles = append(rec.BodyFiles, BodyFile{
			Kind: b.kind, File: rel, Size: int64(len(b.data)), SHA256: hex.EncodeToString(sum[:]),
			ContentType: b.ctype, Truncated: b.capped, OriginalLen: b.full,
		})
	}
	line, err := common.Marshal(rec)
	if err != nil {
		p.setErr(err)
		return
	}
	line = append(line, '\n')
	bucket := t.UTC().Minute() / 10 * 10
	shard := filepath.Join(base, filepath.FromSlash(jsonlRel), twoDigits(bucket)+"-"+p.instance+".jsonl")
	if err := writeFileRetry(shard, line, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640); err != nil {
		p.setErr(err)
		return
	}
	p.written.Add(1)
}

func twoDigits(n int) string {
	return string([]byte{byte('0' + n/10), byte('0' + n%10)})
}
