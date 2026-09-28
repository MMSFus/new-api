package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/sessionrecorder"

	"github.com/gin-gonic/gin"
)

func currentSessionRecorderSettings() (sessionrecorder.Settings, error) {
	common.OptionMapRWMutex.RLock()
	raw := common.OptionMap[sessionrecorder.OptionKey]
	common.OptionMapRWMutex.RUnlock()
	return sessionrecorder.ParseSettings(raw)
}

// GetSessionRecorderSetting returns the recorder settings (never credentials).
func GetSessionRecorderSetting(c *gin.Context) {
	settings, err := currentSessionRecorderSettings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, settings)
}

// UpdateSessionRecorderSetting validates, persists and hot-applies settings.
func UpdateSessionRecorderSetting(c *gin.Context) {
	settings := sessionrecorder.DefaultSettings()
	if err := common.DecodeJson(c.Request.Body, &settings); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid request body"})
		return
	}
	if settings.InstallationID == "" {
		previous, _ := currentSessionRecorderSettings()
		settings.InstallationID = previous.InstallationID
	}
	if settings.InstallationID == "" {
		id, err := sessionrecorder.NewInstallationID()
		if err != nil {
			common.ApiError(c, err)
			return
		}
		settings.InstallationID = id
	}
	if err := settings.Validate(); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	raw, err := common.Marshal(settings)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.UpdateOption(sessionrecorder.OptionKey, string(raw)); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := sessionrecorder.Apply(string(raw)); err != nil {
		common.SysError("session recorder apply failed: " + err.Error())
	}
	common.ApiSuccess(c, settings)
}

// GetSessionRecorderStatus reports producer/consumer runtime state.
func GetSessionRecorderStatus(c *gin.Context) {
	settings, _ := currentSessionRecorderSettings()
	common.ApiSuccess(c, sessionrecorder.Status(settings))
}

// CheckSessionRecorder runs the R2 connectivity probe.
func CheckSessionRecorder(c *gin.Context) {
	settings, err := currentSessionRecorderSettings()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	state, err := sessionrecorder.RunCheck(ctx, settings)
	result := gin.H{"credential_state": state, "state": "ok"}
	if err != nil {
		result["state"] = "failed"
		result["error"] = err.Error()
	}
	common.ApiSuccess(c, result)
}
