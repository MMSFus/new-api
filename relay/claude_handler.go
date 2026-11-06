package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/pkg/k01fixture"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
)

func ClaudeHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {

	info.InitChannelMeta(c)

	claudeReq, ok := info.Request.(*dto.ClaudeRequest)

	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected *dto.ClaudeRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(claudeReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to ClaudeRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	if err := helper.ApplyReasoningModelSuffix(c, info, request); err != nil {
		return newConvertRequestFailedError(c, info, err)
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	if info.ChannelType == constant.ChannelTypeAnthropic && info.ChannelSetting.Tmp20261106K01Enabled {
		storage, storageErr := common.GetBodyStorage(c)
		if storageErr != nil {
			logger.LogError(c, common.LogText("K01 replay could not read request body: %s", storageErr.Error()))
		} else if requestBody, bodyErr := storage.Bytes(); bodyErr != nil {
			logger.LogError(c, common.LogText("K01 replay could not read request body: %s", bodyErr.Error()))
		} else {
			replay, matched, shouldLog, replayErr := k01fixture.Lookup(os.Getenv(k01fixture.FixtureFileEnv), requestBody)
			if replayErr != nil {
				if shouldLog {
					logger.LogError(c, common.LogText("K01 replay fixture load failed: %s", replayErr.Error()))
				}
			} else if matched {
				info.Tmp20261106K01Hit = true
				info.IsStream = replay.IsStream
				c.Header("X-New-Api-Replay", "k01")
				if replay.Status != http.StatusOK {
					c.Header("Content-Type", "application/json")
					c.Status(replay.Status)
					_, _ = c.Writer.Write(replay.Body)
					c.Set("k01_replay_error_written", true)
					return types.NewErrorWithStatusCode(fmt.Errorf("K01 replay returned recorded status %d", replay.Status), types.ErrorCodeBadResponseStatusCode, replay.Status, types.ErrOptionWithSkipRetry())
				}
				contentType := "application/json"
				if replay.IsStream {
					contentType = "text/event-stream"
				}
				httpResp := &http.Response{
					StatusCode:    http.StatusOK,
					Header:        http.Header{"Content-Type": []string{contentType}},
					Body:          io.NopCloser(bytes.NewReader(replay.Body)),
					ContentLength: int64(len(replay.Body)),
				}
				usage, responseErr := adaptor.DoResponse(c, httpResp, info)
				if responseErr != nil {
					service.ResetStatusCode(responseErr, c.GetString("status_code_mapping"))
					return responseErr
				}
				claudeUsage, ok := usage.(*dto.Usage)
				if !ok {
					return types.NewError(fmt.Errorf("K01 replay response usage has unexpected type %T", usage), types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
				}
				service.PostTextConsumeQuota(c, info, claudeUsage, nil)
				return nil
			}
		}
	}

	if info.ChannelSetting.SystemPrompt != "" {
		if request.System == nil {
			request.SetStringSystem(info.ChannelSetting.SystemPrompt)
		} else if info.ChannelSetting.SystemPromptOverride {
			common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
			if request.IsStringSystem() {
				existing := strings.TrimSpace(request.GetStringSystem())
				if existing == "" {
					request.SetStringSystem(info.ChannelSetting.SystemPrompt)
				} else {
					request.SetStringSystem(info.ChannelSetting.SystemPrompt + "\n" + existing)
				}
			} else {
				systemContents := request.ParseSystem()
				newSystem := dto.ClaudeMediaMessage{Type: dto.ContentTypeText}
				newSystem.SetText(info.ChannelSetting.SystemPrompt)
				if len(systemContents) == 0 {
					request.System = []dto.ClaudeMediaMessage{newSystem}
				} else {
					request.System = append([]dto.ClaudeMediaMessage{newSystem}, systemContents...)
				}
			}
		}
	}

	if !model_setting.GetGlobalSettings().PassThroughRequestEnabled &&
		!info.ChannelSetting.PassThroughBodyEnabled &&
		info.ChannelType != constant.ChannelTypeKimi &&
		service.ShouldChatCompletionsUseResponsesGlobal(info.ChannelId, info.ChannelType, info.OriginModelName) {
		usage, newApiErr := textRequestViaResponses(c, info, adaptor, request)
		if newApiErr != nil {
			return newApiErr
		}

		service.PostTextConsumeQuota(c, info, usage, nil)
		return nil
	}

	var requestBody io.Reader
	if info.ChannelType != constant.ChannelTypeKimi && (model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled) {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		requestBody = common.NewReplayableBodyReader(storage)
	} else {
		convertedRequest, err := adaptor.ConvertClaudeRequest(c, info, request)
		if err != nil {
			return newConvertRequestFailedError(c, info, err)
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)
		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// remove disabled fields for Claude API
		jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// apply param override
		if len(info.ParamOverride) > 0 {
			jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
			if err != nil {
				return newAPIErrorFromParamOverride(err)
			}
		}

		logger.LogDebug(c, "requestBody: %s", jsonData)
		body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		defer closer.Close()
		jsonData = nil
		requestBody = body
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")
	var httpResp *http.Response
	simulatedCache := service.PrepareSimulatedCache(c, info)
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return newAPIError
		}
	}
	simulatedCache.WrapResponse(httpResp)

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	claudeUsage, ok := usage.(*dto.Usage)
	if !ok {
		return types.NewError(fmt.Errorf("Claude response usage has unexpected type %T", usage), types.ErrorCodeBadResponseBody, types.ErrOptionWithSkipRetry())
	}
	service.PostTextConsumeQuota(c, info, claudeUsage, nil)
	return nil
}
