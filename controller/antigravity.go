package controller

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/antigravity"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const antigravityOAuthPurpose = "antigravity_channel"

type antigravityOAuthPayload struct {
	Verifier string                    `json:"verifier"`
	Proxy    string                    `json:"proxy"`
	Identity model.AuthSessionIdentity `json:"identity"`
}

func StartAntigravityOAuth(c *gin.Context) {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok || strings.HasPrefix(identity.SessionID, "pat:") {
		common.ApiErrorStatus(c, http.StatusUnauthorized, common.NewMessage("A dashboard session is required"))
		return
	}
	if err := antigravity.ValidateOAuthConfig(); err != nil {
		common.ApiError(c, err)
		return
	}
	var req struct {
		Proxy string `json:"proxy"`
	}
	if c.ShouldBindJSON(&req) != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	if _, err := service.AntigravityHTTPClient(dto.ChannelSettings{Proxy: req.Proxy}); err != nil {
		common.ApiErrorT(c, "Invalid proxy address")
		return
	}
	verifier, challenge, err := antigravity.PKCE()
	if err != nil {
		common.ApiErrorT(c, "Failed to start Antigravity OAuth")
		return
	}
	payload := antigravityOAuthPayload{Verifier: verifier, Proxy: req.Proxy, Identity: model.AuthSessionIdentity{UserID: identity.UserID, SessionID: identity.SessionID, UserAuthVersion: identity.UserAuthVersion, SessionVersion: identity.SessionVersion}}
	encoded, err := common.Marshal(payload)
	if err != nil {
		common.ApiErrorT(c, "Failed to start Antigravity OAuth")
		return
	}
	expiry := time.Now().Add(10 * time.Minute)
	state, _, err := model.CreateAuthFlow(model.AuthFlowCreate{Purpose: antigravityOAuthPurpose, Provider: "antigravity", Intent: "channel", UserId: identity.UserID, SessionId: identity.SessionID, Payload: string(encoded), ExpiresAt: expiry})
	if err != nil {
		common.ApiErrorT(c, "Failed to start Antigravity OAuth")
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"authorization_url": antigravity.AuthorizationURL(state, challenge), "expires_at": expiry.Unix()}})
}

func ExchangeAntigravityOAuth(c *gin.Context) {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok || strings.HasPrefix(identity.SessionID, "pat:") {
		common.ApiErrorStatus(c, http.StatusUnauthorized, common.NewMessage("A dashboard session is required"))
		return
	}
	var req struct {
		CallbackURL string `json:"callback_url"`
	}
	if c.ShouldBindJSON(&req) != nil || len(req.CallbackURL) > 16384 {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	code, state, err := antigravity.Callback(req.CallbackURL)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var payload antigravityOAuthPayload
	_, err = model.ConsumeAuthFlowWithAction(state, model.AuthFlowMatch{Purpose: antigravityOAuthPurpose, Provider: "antigravity", Intent: "channel", UserId: identity.UserID, SessionId: identity.SessionID}, func(tx *gorm.DB, flow *model.AuthFlow) error {
		if common.UnmarshalJsonStr(flow.Payload, &payload) != nil || payload.Verifier == "" {
			return model.ErrAuthFlowInvalid
		}
		if payload.Identity.UserAuthVersion != identity.UserAuthVersion || payload.Identity.SessionVersion != identity.SessionVersion {
			return model.ErrAuthFlowInvalid
		}
		return model.ValidateAuthSessionWithTx(tx, payload.Identity)
	})
	if err != nil {
		common.ApiErrorT(c, "Antigravity authorization has expired or was already used")
		return
	}
	client, err := service.AntigravityHTTPClient(dto.ChannelSettings{Proxy: payload.Proxy})
	if err != nil {
		common.ApiErrorT(c, "Invalid proxy address")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	credential, err := antigravity.Exchange(ctx, client, code, payload.Verifier)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if credential.RefreshToken == "" {
		common.ApiError(c, errors.New("Google did not return a refresh token; authorize again with consent"))
		return
	}
	credential.ProjectID, err = antigravity.DiscoverProject(ctx, client, antigravity.DefaultBaseURL, credential)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"key": credential.Key(), "count": 1}})
}

func ImportAntigravityCredentials(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, antigravity.MaxImportBytes+1024)
	var req struct {
		JSON string `json:"json"`
	}
	if c.ShouldBindJSON(&req) != nil {
		common.ApiErrorT(c, "Invalid parameters")
		return
	}
	key, count, err := service.NormalizeAntigravityKeys(req.JSON)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"key": key, "count": count}})
}
