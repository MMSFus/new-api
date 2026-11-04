package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/sessionrecorder"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type trackedBodyStorage struct {
	data   []byte
	reader *trackedBodyReader
}

func (s *trackedBodyStorage) Read(p []byte) (int, error)     { return 0, io.EOF }
func (s *trackedBodyStorage) Seek(int64, int) (int64, error) { return 0, nil }
func (s *trackedBodyStorage) Close() error                   { return nil }
func (s *trackedBodyStorage) Bytes() ([]byte, error)         { return s.data, nil }
func (s *trackedBodyStorage) Size() int64                    { return int64(len(s.data)) }
func (s *trackedBodyStorage) IsDisk() bool                   { return true }
func (s *trackedBodyStorage) NewReader() (io.ReadCloser, error) {
	s.reader = &trackedBodyReader{Reader: bytes.NewReader(s.data)}
	return s.reader, nil
}

type trackedBodyReader struct {
	*bytes.Reader
	reads int
}

func (r *trackedBodyReader) Read(p []byte) (int, error) {
	r.reads++
	return r.Reader.Read(p)
}

func (r *trackedBodyReader) Close() error { return nil }

type deadlineGinWriter struct {
	gin.ResponseWriter
	deadline time.Time
}

func (w *deadlineGinWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

func newRecorderSnapshot(t *testing.T, filter sessionrecorder.FilterConfig) *sessionrecorder.Snapshot {
	t.Helper()
	compiled, err := sessionrecorder.CompileFilter(filter)
	require.NoError(t, err)
	settings := sessionrecorder.Settings{
		SpoolRoot:          t.TempDir(),
		QueueSize:          2,
		MaxBodyBytes:       1 << 20,
		RecordResponseBody: true,
		RecordRequestBody:  true,
	}
	return &sessionrecorder.Snapshot{
		Settings: settings,
		Filter:   compiled,
		Producer: sessionrecorder.NewProducer(settings),
	}
}

func TestRecorderWriterUnwrapsResponseWriter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	underlying := &deadlineGinWriter{ResponseWriter: c.Writer}
	w := &recorderWriter{ResponseWriter: underlying, c: c, snap: newRecorderSnapshot(t, sessionrecorder.FilterConfig{})}
	deadline := time.Now()

	require.Same(t, underlying, w.Unwrap())
	require.NoError(t, http.NewResponseController(w).SetWriteDeadline(deadline))
	require.Equal(t, deadline, underlying.deadline)
}

func TestSessionRecorderUsesFinalRelayFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Set(string(constant.ContextKeyChannelId), 10)
	c.Set(string(constant.ContextKeyUsingGroup), "group-a")
	c.Set(string(constant.ContextKeyOriginalModel), "model-a")
	snap := newRecorderSnapshot(t, sessionrecorder.FilterConfig{
		Exclude: []sessionrecorder.FilterRule{{ChannelID: 20}},
	})
	w := &recorderWriter{ResponseWriter: c.Writer, c: c, snap: snap}

	_, err := w.Write([]byte(": keep-alive\n\n"))
	require.NoError(t, err)
	c.Set(string(constant.ContextKeyChannelId), 20)
	finishRecording(c, w, time.Now())

	require.EqualValues(t, 0, snap.Producer.Counters().Enqueued)
	require.EqualValues(t, 0, snap.Producer.Counters().InflightBytes)
}

func TestSessionRecorderQueuesRequestReaderWithoutReadingIt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	c.Set(string(constant.ContextKeyChannelId), 10)
	storage := &trackedBodyStorage{data: []byte("request body")}
	c.Set(common.KeyBodyStorage, storage)
	snap := newRecorderSnapshot(t, sessionrecorder.FilterConfig{})
	w := &recorderWriter{ResponseWriter: c.Writer, c: c, snap: snap}

	finishRecording(c, w, time.Now())

	require.NotNil(t, storage.reader)
	require.Zero(t, storage.reader.reads)
	require.EqualValues(t, 1, snap.Producer.Counters().Enqueued)
	require.EqualValues(t, int64(len(storage.data)), snap.Producer.Counters().InflightBytes)
}

func TestSessionRecorderDoesNotSwallowBusinessPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	snap := newRecorderSnapshot(t, sessionrecorder.FilterConfig{})
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(func(c *gin.Context) { sessionRecorderWithSnapshot(c, snap) })
	router.GET("/panic", func(*gin.Context) { panic("relay failed") })
	response := httptest.NewRecorder()

	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))

	require.Equal(t, http.StatusInternalServerError, response.Code)
}
