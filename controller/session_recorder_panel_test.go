package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/sessionrecorder"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const sessionRecorderBodyRecordPath = "gpt-image-2/jsonl/2026/11/05/10/00-ab46d8fc.jsonl"

// setupSessionRecorderBody stores one indexed record whose spool files hold
// the given response body, and points the recorder settings at that spool.
func setupSessionRecorderBody(t *testing.T, response, contentType string) *gin.Engine {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, i18n.Init())
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AuditLog{}, &model.SessionRecorderRecord{}))
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false

	root := t.TempDir()
	ref := "body/2026/11/05/10/ab/cd/sess-1/rec-1_response.body"
	bodyPath := filepath.Join(root, "gpt-image-2", filepath.FromSlash(ref))
	require.NoError(t, os.MkdirAll(filepath.Dir(bodyPath), 0o755))
	require.NoError(t, os.WriteFile(bodyPath, []byte(response), 0o644))
	line, err := common.Marshal(sessionrecorder.Record{
		SchemaVersion: 1, RecordID: "rec-1", SessionID: "sess-1", Model: "gpt-image-2", Path: "/v1/images/generations", StatusCode: 200,
		Attachments: []sessionrecorder.Attachment{},
		BodyFiles:   []sessionrecorder.BodyFile{{Kind: "response", File: ref, Size: int64(len(response)), ContentType: contentType, Truncated: true}},
	})
	require.NoError(t, err)
	recordPath := filepath.Join(root, filepath.FromSlash(sessionRecorderBodyRecordPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(recordPath), 0o755))
	require.NoError(t, os.WriteFile(recordPath, append(line, '\n'), 0o644))
	require.NoError(t, db.Create(&model.SessionRecorderRecord{
		RecordId: "rec-1", SessionId: "sess-1", ModelName: "gpt-image-2", RootId: "primary", RecordPath: sessionRecorderBodyRecordPath,
	}).Error)

	settings := sessionrecorder.DefaultSettings()
	settings.SpoolRoot, settings.RootID = root, "primary"
	raw, err := common.Marshal(settings)
	require.NoError(t, err)
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{sessionrecorder.OptionKey: string(raw)}
	common.OptionMapRWMutex.Unlock()

	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.RedisEnabled = previousRedis
	})

	router := gin.New()
	router.GET("/api/session_recorder/records/:record_id/body", func(c *gin.Context) {
		c.Set("id", 1)
		c.Set("role", common.RoleAdminUser)
		c.Set("username", "reviewer")
	}, GetSessionRecorderRecordBody)
	return router
}

type sessionRecorderBodyAuditParams struct {
	RecordID  string `json:"record_id"`
	SessionID string `json:"session_id"`
	Kind      string `json:"kind"`
}

// requireSingleBodyAudit asserts the download wrote exactly one audit row.
func requireSingleBodyAudit(t *testing.T) (model.AuditLog, sessionRecorderBodyAuditParams) {
	t.Helper()
	var logs []model.AuditLog
	require.NoError(t, model.LOG_DB.Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, "session_recorder.download_body", logs[0].Action)
	require.NotNil(t, logs[0].Other.Op)
	raw, err := common.Marshal(logs[0].Other.Op.Params)
	require.NoError(t, err)
	var params sessionRecorderBodyAuditParams
	require.NoError(t, common.Unmarshal(raw, &params))
	return logs[0], params
}

func TestGetSessionRecorderRecordBodyStreamsTheStoredBody(t *testing.T) {
	response := `{"data":[{"b64_json":"AAAA"},{"b64_json":"BBBB"}]}`
	router := setupSessionRecorderBody(t, response, "application/json; charset=utf-8")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/session_recorder/records/rec-1/body?kind=response", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, response, recorder.Body.String())
	assert.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
	assert.Equal(t, strconv.Itoa(len(response)), recorder.Header().Get("Content-Length"))
	assert.Equal(t, "attachment; filename=rec-1-response", recorder.Header().Get("Content-Disposition"))
	assert.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "true", recorder.Header().Get("X-Recorder-Truncated"))

	audit, params := requireSingleBodyAudit(t)
	assert.True(t, audit.Success)
	assert.Equal(t, http.StatusOK, audit.Status)
	assert.Equal(t, sessionRecorderBodyAuditParams{RecordID: "rec-1", SessionID: "sess-1", Kind: "response"}, params)
}

func TestGetSessionRecorderRecordBodyServesAMalformedContentTypeAsOctetStream(t *testing.T) {
	router := setupSessionRecorderBody(t, "<svg/>", "text/html; charset")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/session_recorder/records/rec-1/body?kind=response", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "application/octet-stream", recorder.Header().Get("Content-Type"))
}

func TestGetSessionRecorderRecordBodyFailuresAreAudited(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		httpStatus int
		message    string
		sessionID  string
	}{
		{name: "invalid kind", path: "/api/session_recorder/records/rec-1/body?kind=bogus", httpStatus: http.StatusBadRequest, message: "Invalid body kind"},
		{name: "unknown record", path: "/api/session_recorder/records/missing/body?kind=response", httpStatus: http.StatusOK, message: "Recorded content is no longer available"},
		{name: "kind without a stored body", path: "/api/session_recorder/records/rec-1/body?kind=request", httpStatus: http.StatusOK, message: "Recorded content is no longer available", sessionID: "sess-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupSessionRecorderBody(t, `{"data":[]}`, "application/json")

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.path, nil))

			require.Equal(t, tt.httpStatus, recorder.Code)
			assert.Empty(t, recorder.Header().Get("X-Recorder-Truncated"))
			var body struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
			assert.False(t, body.Success)
			assert.Equal(t, tt.message, body.Message)

			audit, params := requireSingleBodyAudit(t)
			assert.False(t, audit.Success)
			assert.Equal(t, tt.httpStatus, audit.Status)
			assert.Equal(t, tt.sessionID, params.SessionID)
		})
	}
}
