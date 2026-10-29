package controller

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/kimi"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const kimiOAuthPurpose = "kimi_channel"
const kimiOAuthPrefix = "kimi-oauth:"

type kimiOAuthState struct {
	Identity   model.AuthSessionIdentity `json:"identity"`
	Credential kimi.Credential           `json:"credential"`
	Proxy      string                    `json:"proxy"`
	DeviceCode string                    `json:"device_code,omitempty"`
	Interval   int                       `json:"interval"`
	NextPoll   int64                     `json:"next_poll"`
	Deadline   int64                     `json:"deadline"`
}

func kimiSession(c *gin.Context) (model.AuthSessionIdentity, error) {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok || strings.HasPrefix(identity.SessionID, "pat:") {
		return identity, errors.New("A dashboard session is required")
	}
	return identity, model.DB.Transaction(func(tx *gorm.DB) error { return model.ValidateAuthSessionWithTx(tx, identity) })
}
func createKimiOAuthState(state kimiOAuthState, intent string) (string, error) {
	b, err := common.Marshal(state)
	if err != nil {
		return "", err
	}
	handle, err := model.CreateKimiAuthFlow(model.AuthFlowCreate{Purpose: kimiOAuthPurpose, Provider: "kimi", Intent: intent, UserId: state.Identity.UserID, SessionId: state.Identity.SessionID, Payload: string(b), ExpiresAt: time.Unix(state.Deadline, 0)}, state.Identity)
	return handle, err
}
func readKimiOAuthState(c *gin.Context, handle, intent string, consume bool) (kimiOAuthState, error) {
	var state kimiOAuthState
	identity, err := kimiSession(c)
	if err != nil {
		return state, err
	}
	match := model.AuthFlowMatch{Purpose: kimiOAuthPurpose, Provider: "kimi", Intent: intent, UserId: identity.UserID, SessionId: identity.SessionID}
	action := func(tx *gorm.DB, flow *model.AuthFlow) error {
		if common.UnmarshalJsonStr(flow.Payload, &state) != nil || state.Identity != identity {
			return model.ErrAuthFlowInvalid
		}
		if intent == "poll" && state.NextPoll > time.Now().Unix() {
			return errors.New("Wait for the Kimi polling interval")
		}
		return model.ValidateAuthSessionWithTx(tx, identity)
	}
	if consume {
		_, err = model.ConsumeAuthFlowWithAction(handle, match, action)
	} else {
		var flow *model.AuthFlow
		flow, err = model.GetAuthFlow(handle, match)
		if err == nil {
			err = model.DB.Transaction(func(tx *gorm.DB) error { return action(tx, flow) })
		}
	}
	return state, err
}
func resolveKimiOAuthReference(c *gin.Context, key string, consume bool) (string, error) {
	handle, ok := strings.CutPrefix(strings.TrimSpace(key), kimiOAuthPrefix)
	if !ok {
		return key, nil
	}
	state, err := readKimiOAuthState(c, handle, "ready", consume)
	if err != nil {
		return "", errors.New("Kimi authorization expired or was already used; authorize again")
	}
	return state.Credential.Key(), nil
}
func StartKimiOAuth(c *gin.Context) {
	identity, err := kimiSession(c)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusUnauthorized, common.NewMessage("A dashboard session is required"))
		return
	}
	var req struct {
		Region string `json:"region"`
		Proxy  string `json:"proxy"`
	}
	if c.ShouldBindJSON(&req) != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	region, err := kimi.NormalizeRegion(req.Region)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	client, err := service.KimiHTTPClient(dto.ChannelSettings{Proxy: req.Proxy})
	if err != nil {
		common.ApiErrorT(c, "Invalid proxy address")
		return
	}
	credential := kimi.Credential{Region: region, DeviceID: common.GetRandomString(32)}
	d, err := kimi.StartDevice(c.Request.Context(), client, credential)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	state := kimiOAuthState{Identity: identity, Credential: credential, Proxy: req.Proxy, DeviceCode: d.DeviceCode, Interval: d.Interval, NextPoll: time.Now().Unix() + int64(d.Interval), Deadline: time.Now().Unix() + int64(d.ExpiresIn)}
	handle, err := createKimiOAuthState(state, "poll")
	if err != nil {
		common.ApiErrorT(c, "Failed to start Kimi OAuth")
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"handle": handle, "user_code": d.UserCode, "verification_uri": d.VerificationURI, "verification_uri_complete": d.VerificationURIComplete, "interval": d.Interval, "expires_at": state.Deadline}})
}
func PollKimiOAuth(c *gin.Context) {
	var req struct {
		Handle string `json:"handle"`
	}
	if c.ShouldBindJSON(&req) != nil || len(req.Handle) > 256 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	state, err := readKimiOAuthState(c, req.Handle, "poll", true)
	if err != nil {
		common.ApiErrorT(c, "Kimi authorization expired, was already used, or was polled too soon")
		return
	}
	client, err := service.KimiHTTPClient(dto.ChannelSettings{Proxy: state.Proxy})
	if err != nil {
		common.ApiErrorT(c, "Invalid proxy address")
		return
	}
	credential, err := kimi.Token(c.Request.Context(), client, state.Credential, state.DeviceCode)
	if err != nil {
		var oauthErr *kimi.OAuthError
		if !errors.As(err, &oauthErr) || (oauthErr.Code != "authorization_pending" && oauthErr.Code != "slow_down") {
			common.ApiError(c, err)
			return
		}
		if oauthErr.Code == "slow_down" {
			state.Interval += 5
		}
		state.NextPoll = time.Now().Unix() + int64(state.Interval)
		handle, err := createKimiOAuthState(state, "poll")
		if err != nil {
			common.ApiErrorT(c, "Kimi authorization expired; authorize again")
			return
		}
		c.JSON(200, gin.H{"success": true, "data": gin.H{"status": "pending", "handle": handle, "interval": state.Interval}})
		return
	}
	state.Credential = credential
	state.DeviceCode = ""
	state.Deadline = time.Now().Add(10 * time.Minute).Unix()
	handle, err := createKimiOAuthState(state, "ready")
	if err != nil {
		common.ApiErrorT(c, "Failed to save Kimi authorization; authorize again")
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"status": "ready", "key": kimiOAuthPrefix + handle, "count": 1}})
}
func CancelKimiOAuth(c *gin.Context) {
	var req struct {
		Handle string `json:"handle"`
	}
	if c.ShouldBindJSON(&req) != nil || len(req.Handle) > 256 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	identity, err := kimiSession(c)
	if err != nil {
		common.ApiErrorT(c, "A dashboard session is required")
		return
	}
	// Cancellation intentionally ignores next_poll but checks identity and session.
	_, err = model.ConsumeAuthFlowWithAction(strings.TrimPrefix(req.Handle, kimiOAuthPrefix), model.AuthFlowMatch{Purpose: kimiOAuthPurpose, Provider: "kimi", UserId: identity.UserID, SessionId: identity.SessionID}, func(tx *gorm.DB, flow *model.AuthFlow) error {
		var state kimiOAuthState
		if common.UnmarshalJsonStr(flow.Payload, &state) != nil || state.Identity != identity {
			return model.ErrAuthFlowInvalid
		}
		return model.ValidateAuthSessionWithTx(tx, identity)
	})
	if err != nil {
		common.ApiErrorT(c, "Kimi authorization expired or was already used")
		return
	}
	common.ApiSuccessT(c, "Authorization cancelled", nil)
}
func ImportKimiCredentials(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2*kimi.MaxImportBytes+1024)
	var req struct {
		JSON   string `json:"json"`
		Region string `json:"region"`
	}
	if c.ShouldBindJSON(&req) != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	credentials, issues, err := kimi.Preview(req.JSON, req.Region)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	lines := make([]string, len(credentials))
	preview := make([]gin.H, len(credentials))
	for i, credential := range credentials {
		lines[i] = credential.Key()
		preview[i] = gin.H{"name": credential.Name, "region": credential.Region, "auth_type": credential.AuthType, "expires_at": credential.ExpiresAt, "renewable": credential.RefreshToken != "", "key": credential.Key()}
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"key": strings.Join(lines, "\n"), "count": len(lines), "accounts": preview, "issues": issues}})
}

// saveKimiOAuthReference consumes a ready result in the same transaction as its
// channel write. Validation or database failures leave the result available.
func saveKimiOAuthReference(c *gin.Context, key string, save func(*gorm.DB) error) error {
	handle, ok := strings.CutPrefix(strings.TrimSpace(key), kimiOAuthPrefix)
	if !ok {
		return save(model.DB)
	}
	identity, err := kimiSession(c)
	if err != nil {
		return err
	}
	_, err = model.ConsumeAuthFlowWithAction(handle, model.AuthFlowMatch{Purpose: kimiOAuthPurpose, Provider: "kimi", Intent: "ready", UserId: identity.UserID, SessionId: identity.SessionID}, func(tx *gorm.DB, flow *model.AuthFlow) error {
		var state kimiOAuthState
		if common.UnmarshalJsonStr(flow.Payload, &state) != nil || state.Identity != identity {
			return model.ErrAuthFlowInvalid
		}
		if err := model.ValidateAuthSessionWithTx(tx, identity); err != nil {
			return err
		}
		return save(tx)
	})
	return err
}

// GetKimiChannelUsage exposes subscription ratios separately from token billing.
func GetKimiChannelUsage(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	ch, err := model.GetChannelById(id, true)
	if err != nil || ch.Type != constant.ChannelTypeKimi {
		common.ApiErrorT(c, "Invalid Kimi channel")
		return
	}
	key, _, apiErr := ch.GetNextEnabledKey()
	if apiErr != nil {
		common.ApiErrorT(c, "No enabled Kimi credential")
		return
	}
	credential, err := service.ResolveKimiCredential(c.Request.Context(), id, key, ch.GetSetting(), "")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	client, err := service.KimiHTTPClient(ch.GetSetting())
	if err != nil {
		common.ApiErrorT(c, "Invalid proxy address")
		return
	}
	raw, err := kimi.Get(c.Request.Context(), client, credential, "/usages")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var payload struct {
		Usages map[string]struct {
			UsedRatio any    `json:"used_ratio"`
			ResetTime string `json:"reset_time"`
		} `json:"usages"`
	}
	if common.Unmarshal(raw, &payload) != nil || payload.Usages == nil {
		common.ApiErrorT(c, "Invalid Kimi usage response")
		return
	}
	entries := make([]gin.H, 0, 4)
	for _, name := range []string{"limit_5h", "limit_7d", "limit_month_total", "limit_month_code"} {
		value, ok := payload.Usages[name]
		if !ok {
			continue
		}
		var ratio float64
		var parseErr error
		switch v := value.UsedRatio.(type) {
		case float64:
			ratio = v
		case string:
			ratio, parseErr = strconv.ParseFloat(v, 64)
		default:
			continue
		}
		if parseErr != nil || math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio < 0 {
			continue
		}
		reset := ""
		if at, parseErr := time.Parse(time.RFC3339, value.ResetTime); parseErr == nil {
			reset = at.Format(time.RFC3339)
		}
		entries = append(entries, gin.H{"name": name, "used_ratio": ratio, "reset_time": reset})
	}
	c.JSON(200, gin.H{"success": true, "data": entries})
}
