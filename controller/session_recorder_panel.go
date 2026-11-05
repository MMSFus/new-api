package controller

import (
	"context"
	"errors"
	"io"
	"mime"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/sessionrecorder"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// sessionRecorderRevenueMaxRange bounds the revenue summary.
const sessionRecorderRevenueMaxRange = 92 * 24 * 60 * 60

func sessionRecorderOption(key string) string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return common.OptionMap[key]
}

func currentRevenueRules() ([]sessionrecorder.RevenueRule, error) {
	return sessionrecorder.ParseRevenueRules(sessionRecorderOption(sessionrecorder.RevenueRulesOptionKey))
}

// GetSessionRecorderRevenueRules returns the ordered revenue rules.
func GetSessionRecorderRevenueRules(c *gin.Context) {
	rules, err := currentRevenueRules()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"rules":          rules,
		"currency_label": sessionRecorderOption(sessionrecorder.RevenueCurrencyLabelOptionKey),
	})
}

type revenueRulesRequest struct {
	Rules         []sessionrecorder.RevenueRule `json:"rules"`
	CurrencyLabel *string                       `json:"currency_label"`
}

// UpdateSessionRecorderRevenueRules validates and stores the rules and, when
// given, the currency label.
func UpdateSessionRecorderRevenueRules(c *gin.Context) {
	var req revenueRulesRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if req.Rules == nil {
		req.Rules = []sessionrecorder.RevenueRule{}
	}
	if _, err := sessionrecorder.CompileRevenueRules(req.Rules); err != nil {
		common.ApiError(c, err)
		return
	}
	raw, err := common.Marshal(req.Rules)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	values := map[string]string{sessionrecorder.RevenueRulesOptionKey: string(raw)}
	if req.CurrencyLabel != nil {
		values[sessionrecorder.RevenueCurrencyLabelOptionKey] = strings.TrimSpace(*req.CurrencyLabel)
	}
	if err := model.UpdateOptionsBulk(values); err != nil {
		common.ApiError(c, err)
		return
	}
	GetSessionRecorderRevenueRules(c)
}

// TestSessionRecorderRevenueRules reports which rule of an unsaved rule list
// a model name would match.
func TestSessionRecorderRevenueRules(c *gin.Context) {
	var req struct {
		Rules []sessionrecorder.RevenueRule `json:"rules"`
		Model string                        `json:"model"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	matcher, err := sessionrecorder.CompileRevenueRules(req.Rules)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result := gin.H{"matched_rule_id": nil, "matched_rule_name": nil}
	if rule, ok := matcher.Match(req.Model); ok {
		result["matched_rule_id"], result["matched_rule_name"] = rule.ID, rule.Name
	}
	common.ApiSuccess(c, result)
}

// GetSessionRecorderRevenueSummary sums archived data and revenue with the
// current rules.
func GetSessionRecorderRevenueSummary(c *gin.Context) {
	start, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	end, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	if start <= 0 || end <= 0 {
		common.ApiErrorT(c, "Start and end time are required")
		return
	}
	if end < start {
		common.ApiErrorT(c, "End time must be after start time")
		return
	}
	if end-start > sessionRecorderRevenueMaxRange {
		common.ApiErrorT(c, "The time range cannot exceed {{days}} days", map[string]any{"days": sessionRecorderRevenueMaxRange / (24 * 60 * 60)})
		return
	}
	bucket := int64(86400)
	switch c.DefaultQuery("granularity", "day") {
	case "day":
	case "hour":
		bucket = 3600
	default:
		common.ApiErrorT(c, "Granularity must be hour or day")
		return
	}
	rules, err := currentRevenueRules()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	matcher, err := sessionrecorder.CompileRevenueRules(rules)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	summary, err := model.SumSessionRecorderRevenue(model.SessionRecorderRevenueFilter{
		StartTimestamp: start, EndTimestamp: end, ModelName: strings.TrimSpace(c.Query("model")), BucketSeconds: bucket,
	}, matcher)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}

// ListSessionRecorderSessions pages recorded sessions by latest activity.
func ListSessionRecorderSessions(c *gin.Context) {
	filter := model.SessionRecorderSessionFilter{
		ModelName: strings.TrimSpace(c.Query("model")),
		SessionId: strings.TrimSpace(c.Query("session_id")),
	}
	filter.StartTimestamp, _ = strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	filter.EndTimestamp, _ = strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	filter.UserId, _ = strconv.Atoi(c.Query("user_id"))
	pageInfo := common.GetPageQuery(c)
	pageSize := min(pageInfo.GetPageSize(), 100)
	items, total, err := model.ListSessionRecorderSessions(filter, (pageInfo.GetPage()-1)*pageSize, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": items, "total": total})
}

// ListSessionRecorderSessionRecords lists one session's records in time
// order. The session id is a query parameter because it may contain "/".
func ListSessionRecorderSessionRecords(c *gin.Context) {
	sessionID := c.Query("session_id")
	if sessionID == "" {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	items, total, err := model.ListSessionRecorderSessionRecords(sessionID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"items": items, "total": total, "limit": model.SessionRecorderSessionRecordLimit})
}

// loadSessionRecorderContent finds a record in the index and reads its
// content from the spool or the R2 archive.
func loadSessionRecorderContent(ctx context.Context, recordID string) (*sessionrecorder.RecordContent, string, error) {
	record, err := model.GetSessionRecorderRecord(recordID)
	if err != nil {
		return nil, "", err
	}
	if record == nil {
		return nil, "", sessionrecorder.ErrRecordContentUnavailable
	}
	settings, err := currentSessionRecorderSettings()
	if err != nil {
		return nil, record.SessionId, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	content, err := sessionrecorder.ReadRecordContent(ctx, settings, sessionrecorder.ContentLocation{
		RecordID: record.RecordId, RootID: record.RootId, RecordPath: record.RecordPath, ObjectKey: record.ObjectKey,
	}, nil)
	return content, record.SessionId, err
}

// GetSessionRecorderRecordContent returns a record with its request and
// response bodies. Every call is audited with the record and session id
// only, never the content.
func GetSessionRecorderRecordContent(c *gin.Context) {
	recordID := c.Param("record_id")
	content, sessionID, err := loadSessionRecorderContent(c.Request.Context(), recordID)
	const action = "session_recorder.view_content"
	params := map[string]any{"record_id": recordID, "session_id": sessionID}
	model.RecordOperationAuditLog(c.GetInt("id"), c.GetInt("role"), auditContentEN(action, params), c.ClientIP(), action, params,
		auditOperatorInfo(c), &model.AuditRequestInfo{Method: c.Request.Method, Route: c.FullPath(), Path: c.FullPath(), Status: 200, Success: err == nil}, c)
	if err != nil {
		common.ApiError(c, sessionRecorderContentError(err))
		return
	}
	common.ApiSuccess(c, content)
}

// GetSessionRecorderRecordBody streams the complete stored request or response body.
func GetSessionRecorderRecordBody(c *gin.Context) {
	recordID, kind := c.Param("record_id"), c.Query("kind")
	var sessionID string
	invalidKind := kind != "request" && kind != "response"
	status, success := 200, false
	const action = "session_recorder.download_body"
	params := map[string]any{"record_id": recordID, "session_id": sessionID, "kind": kind}
	defer func() {
		params["session_id"] = sessionID
		model.RecordOperationAuditLog(c.GetInt("id"), c.GetInt("role"), auditContentEN(action, params), c.ClientIP(), action, params,
			auditOperatorInfo(c), &model.AuditRequestInfo{Method: c.Request.Method, Route: c.FullPath(), Path: c.FullPath(), Status: status, Success: success}, c)
	}()
	if invalidKind {
		status = 400
		common.ApiErrorStatus(c, status, common.NewMessage("Invalid body kind"))
		return
	}
	record, err := model.GetSessionRecorderRecord(recordID)
	if err != nil {
		common.ApiError(c, sessionRecorderContentError(err))
		return
	}
	if record == nil {
		common.ApiError(c, sessionrecorder.ErrRecordContentUnavailable)
		return
	}
	sessionID = record.SessionId
	settings, err := currentSessionRecorderSettings()
	if err != nil {
		common.ApiError(c, sessionRecorderContentError(err))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()
	bodyStarted := false
	_, _, err = sessionrecorder.WriteRecordBody(ctx, settings, sessionrecorder.ContentLocation{
		RecordID: record.RecordId, RootID: record.RootId, RecordPath: record.RecordPath, ObjectKey: record.ObjectKey,
	}, nil, kind, func(body sessionrecorder.BodyFile, size int64) io.Writer {
		bodyStarted = true
		contentType := body.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		if _, _, parseErr := mime.ParseMediaType(contentType); parseErr != nil {
			contentType = "application/octet-stream"
		}
		c.Header("Content-Type", contentType)
		c.Header("Content-Length", strconv.FormatInt(size, 10))
		c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": recordID + "-" + kind}))
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Recorder-Truncated", strconv.FormatBool(body.Truncated))
		c.Status(200)
		return c.Writer
	})
	if err != nil {
		if bodyStarted {
			common.SysError("session recorder body stream failed: " + err.Error())
			return
		}
		common.ApiError(c, sessionRecorderContentError(err))
		return
	}
	success = true
}

func sessionRecorderContentError(err error) error {
	var message *common.Message
	if errors.As(err, &message) {
		return err
	}
	common.SysError("session recorder content read failed: " + err.Error())
	return common.NewMessage("Recorded content could not be read")
}

// sessionRecorderBackfillResponse flattens the latest backfill task.
type sessionRecorderBackfillResponse struct {
	State  string `json:"state"`
	TaskID string `json:"task_id"`
	service.SessionRecorderBackfillState
	StartedAt  int64 `json:"started_at"`
	FinishedAt int64 `json:"finished_at"`
}

func sessionRecorderBackfillStatus(task *model.SystemTask) sessionRecorderBackfillResponse {
	response := sessionRecorderBackfillResponse{State: "idle"}
	if task == nil {
		return response
	}
	_ = task.DecodeState(&response.SessionRecorderBackfillState)
	response.TaskID, response.StartedAt = task.TaskID, task.CreatedAt
	switch task.Status {
	case model.SystemTaskStatusPending, model.SystemTaskStatusRunning:
		response.State = "running"
		return response
	case model.SystemTaskStatusSucceeded:
		response.State = "done"
	default:
		response.State = "failed"
		if response.Cancelled {
			response.State = "cancelled"
		}
		if response.LastError == "" {
			response.LastError = task.Error
		}
	}
	response.FinishedAt = task.UpdatedAt
	return response
}

// CreateSessionRecorderBackfill starts the historical index backfill, or
// returns the one already running.
func CreateSessionRecorderBackfill(c *gin.Context) {
	task, _, err := service.EnqueueSystemTask(model.SystemTaskTypeSessionRecorderBackfill, struct{}{})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, sessionRecorderBackfillStatus(task))
}

func GetSessionRecorderBackfill(c *gin.Context) {
	task, err := model.GetLatestSystemTask(model.SystemTaskTypeSessionRecorderBackfill)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, sessionRecorderBackfillStatus(task))
}

// CancelSessionRecorderBackfill asks the running backfill to stop after the
// current batch. A later start resumes from the checkpoint.
func CancelSessionRecorderBackfill(c *gin.Context) {
	if _, err := model.RequestSystemTaskCancel(model.SystemTaskTypeSessionRecorderBackfill); err != nil {
		common.ApiError(c, err)
		return
	}
	GetSessionRecorderBackfill(c)
}
