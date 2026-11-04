package middleware

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/sessionrecorder"

	"github.com/gin-gonic/gin"
)

// recordedHeaders is an allow-list: credentials (Authorization, x-api-key,
// cookies, x-goog-api-key, ...) are never recorded.
var recordedHeaders = []string{
	"Content-Type", "Accept", "User-Agent", "Anthropic-Version", "Anthropic-Beta",
	"Openai-Beta", "X-Stainless-Lang", "X-Stainless-Package-Version", "X-Stainless-Runtime",
	"X-Stainless-Runtime-Version", "X-Stainless-Os", "X-Stainless-Arch", "X-App",
	"Session_id", "X-Session-Id", "X-Claude-Code-Session-Id",
}

var sessionHeaders = []string{"Session_id", "X-Session-Id", "X-Claude-Code-Session-Id"}

const recorderReserveChunk = 64 << 10

type recorderWriter struct {
	gin.ResponseWriter
	c           *gin.Context
	snap        *sessionrecorder.Snapshot
	decision    sessionrecorder.RequestInfo
	hasDecision bool
	matched     bool
	capture     bool
	capped      bool
	body        []byte
	full        int64
	reserved    int64
}

func (w *recorderWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *recorderWriter) decide() {
	info := sessionrecorder.RequestInfo{
		ChannelID: common.GetContextKeyInt(w.c, constant.ContextKeyChannelId),
		Group:     common.GetContextKeyString(w.c, constant.ContextKeyUsingGroup),
		Model:     common.GetContextKeyString(w.c, constant.ContextKeyOriginalModel),
	}
	if w.hasDecision && w.decision == info {
		return
	}
	if w.hasDecision {
		w.snap.Producer.Release(w.reserved)
		w.reserved = 0
		w.body = nil
		w.full = 0
		w.capped = false
	}
	w.decision = info
	w.hasDecision = true
	w.matched = false
	defer func() {
		if recover() != nil {
			w.matched = false
			w.capture = false
		}
	}()
	if info.ChannelID <= 0 {
		w.capture = false
		return
	}
	w.matched = w.snap.Filter.Match(info)
	w.capture = w.matched && w.snap.Settings.RecordResponseBody && w.snap.Settings.MaxBodyBytes > 0
}

func (w *recorderWriter) tee(p []byte) {
	w.full += int64(len(p))
	if !w.capture || w.capped {
		return
	}
	room := w.snap.Settings.MaxBodyBytes - int64(len(w.body))
	take := min(int64(len(p)), room)
	if take < int64(len(p)) {
		w.capped = true
	}
	if take <= 0 {
		return
	}
	for int64(len(w.body))+take > w.reserved {
		if !w.snap.Producer.Reserve(recorderReserveChunk) {
			w.capped = true
			take = w.reserved - int64(len(w.body))
			break
		}
		w.reserved += recorderReserveChunk
	}
	if take > 0 {
		w.body = append(w.body, p[:take]...)
	}
}

func (w *recorderWriter) Write(p []byte) (int, error) {
	w.decide()
	n, err := w.ResponseWriter.Write(p)
	if n > 0 && w.matched {
		w.safeTee(p[:n])
	}
	return n, err
}

func (w *recorderWriter) WriteString(s string) (int, error) {
	w.decide()
	n, err := w.ResponseWriter.WriteString(s)
	if n > 0 && w.matched {
		w.safeTee([]byte(s[:n]))
	}
	return n, err
}

func (w *recorderWriter) safeTee(p []byte) {
	defer func() {
		if recover() != nil {
			w.snap.Producer.Release(w.reserved)
			w.reserved = 0
			w.body = nil
			w.capture = false
			w.matched = false
			common.SysError("session recorder capture panic recovered")
		}
	}()
	w.tee(p)
}

// SessionRecorder captures matching relay traffic into the session-recorder
// spool. It does nothing unless the recorder and its producer are enabled,
// never blocks on I/O (captures are handed to a bounded async queue) and
// swallows every recording error or panic.
func SessionRecorder() gin.HandlerFunc {
	return func(c *gin.Context) {
		snap := sessionrecorder.Current()
		if snap == nil || snap.Producer == nil || snap.Filter == nil ||
			c.Request.Method == http.MethodOptions || strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
			c.Next()
			return
		}
		sessionRecorderWithSnapshot(c, snap)
	}
}

func sessionRecorderWithSnapshot(c *gin.Context, snap *sessionrecorder.Snapshot) {
	started := time.Now()
	w := &recorderWriter{ResponseWriter: c.Writer, c: c, snap: snap}
	c.Writer = w
	defer func() { snap.Producer.Release(w.reserved) }()
	c.Next()
	func() {
		defer func() {
			if recover() != nil {
				common.SysError("session recorder capture panic recovered")
			}
		}()
		finishRecording(c, w, started)
	}()
}

func finishRecording(c *gin.Context, w *recorderWriter, started time.Time) {
	w.decide()
	producer := w.snap.Producer
	if !w.matched {
		producer.Release(w.reserved)
		return
	}
	settings := w.snap.Settings
	finished := time.Now()
	requestID := c.GetString(common.RequestIdKey)
	if requestID == "" {
		requestID = common.GetTimeString() + common.GetRandomString(8)
	}
	sessionID := requestID
	for _, h := range sessionHeaders {
		if v := strings.TrimSpace(c.GetHeader(h)); v != "" {
			sessionID = v
			break
		}
	}
	headers := make(map[string]string)
	for _, h := range recordedHeaders {
		if v := c.GetHeader(h); v != "" {
			headers[strings.ToLower(h)] = v
		}
	}
	responseType := w.Header().Get("Content-Type")
	capture := &sessionrecorder.Capture{
		Record: sessionrecorder.Record{
			SchemaVersion:  sessionrecorder.RecordSchemaVersion,
			RecordID:       requestID,
			SessionID:      sessionID,
			Timestamp:      finished.UTC().Format(time.RFC3339Nano),
			StartedAt:      started.UTC().Format(time.RFC3339Nano),
			DurationMs:     finished.Sub(started).Milliseconds(),
			SiteID:         settings.SiteID,
			HostID:         settings.HostID,
			Method:         c.Request.Method,
			Path:           c.Request.URL.Path,
			Stream:         strings.HasPrefix(responseType, "text/event-stream"),
			StatusCode:     w.Status(),
			ChannelID:      common.GetContextKeyInt(c, constant.ContextKeyChannelId),
			ChannelName:    common.GetContextKeyString(c, constant.ContextKeyChannelName),
			ChannelType:    common.GetContextKeyInt(c, constant.ContextKeyChannelType),
			Group:          common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
			Model:          common.GetContextKeyString(c, constant.ContextKeyOriginalModel),
			UserID:         common.GetContextKeyInt(c, constant.ContextKeyUserId),
			TokenID:        common.GetContextKeyInt(c, constant.ContextKeyTokenId),
			RequestHeaders: headers,
			Attachments:    []sessionrecorder.Attachment{},
		},
		ResponseBody:    w.body,
		ResponseFull:    w.full,
		ResponseType:    responseType,
		ResponseCapped:  w.capped,
		IncludeResponse: settings.RecordResponseBody,
		IncludeRequest:  settings.RecordRequestBody,
		FinishedAt:      finished,
	}
	reserved := w.reserved
	if settings.RecordRequestBody {
		capture.RequestType = c.GetHeader("Content-Type")
		reader, full, capped, extra := readRequestBody(c, producer, settings.MaxBodyBytes)
		reserved += extra
		capture.RequestBodyReader = reader
		capture.RequestBodyLimit = min(full, settings.MaxBodyBytes)
		capture.RequestFull, capture.RequestCapped = full, capped
	}
	w.body = nil
	w.reserved = 0
	producer.Enqueue(capture, reserved)
}

// readRequestBody reserves space and opens an independent reader; the producer
// worker copies at most limit bytes after the capture is queued.
func readRequestBody(c *gin.Context, producer *sessionrecorder.Producer, limit int64) (io.ReadCloser, int64, bool, int64) {
	value, ok := c.Get(common.KeyBodyStorage)
	if !ok || value == nil {
		return nil, 0, false, 0
	}
	storage, ok := value.(common.BodyStorage)
	if !ok {
		return nil, 0, false, 0
	}
	full := storage.Size()
	take := min(full, limit)
	if take <= 0 {
		return nil, full, full > 0, 0
	}
	if !producer.Reserve(take) {
		producer.NoteBudgetDrop()
		return nil, full, true, 0
	}
	reader, err := storage.NewReader()
	if err != nil {
		producer.Release(take)
		return nil, full, true, 0
	}
	return reader, full, take < full, take
}
