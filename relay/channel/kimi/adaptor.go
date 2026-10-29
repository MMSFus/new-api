package kimi

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"slices"

	"github.com/QuantumNous/new-api/common"

	"github.com/QuantumNous/new-api/pkg/kimi"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// Adaptor uses the regional Coding API. Web-session tokens are never sent to
// either the Coding resource server or its OAuth token endpoint.
type Adaptor struct {
	credential kimi.Credential
	target     types.RelayFormat
	metadata   kimi.Model
}

func (a *Adaptor) Init(*relaycommon.RelayInfo) {}
func (a *Adaptor) convert(c *gin.Context, info *relaycommon.RelayInfo, request any) (_ any, err error) {
	credential, models, err := service.ResolveKimiModels(c.Request.Context(), info.ChannelId, info.ApiKey, info.ChannelSetting)
	if err != nil {
		return nil, err
	}
	if err := service.ValidateKimiBase(info.ChannelBaseUrl, credential); err != nil {
		return nil, err
	}
	a.credential = credential
	found := false
	for _, model := range models {
		if model.ID == info.UpstreamModelName {
			a.metadata = model
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("Kimi model is unavailable; fetch models and update the channel mapping")
	}
	switch a.metadata.Protocol {
	case "", "kimi", "openai":
		a.target = types.RelayFormatOpenAI
	case "anthropic":
		a.target = types.RelayFormatClaude
	case "response", "openai_responses":
		a.target = types.RelayFormatOpenAIResponses
	default:
		return nil, errors.New("Unsupported Kimi model protocol")
	}
	if a.target == types.RelayFormatOpenAI && info.RelayFormat == types.RelayFormatOpenAIResponses {
		return nil, errors.New("This Kimi model uses Chat; select a Responses or Anthropic model for Responses requests")
	}
	if a.target != info.RelayFormat {
		// Existing converters enforce the configured loss policy and record diagnostics.
		converted, err := service.ConvertRequest(c, info, a.target, request)
		if err != nil {
			return nil, err
		}
		request = converted.Value
	}
	if err := a.validateCapabilities(request); err != nil {
		return nil, err
	}
	info.FinalRequestRelayFormat = a.target
	return request, nil
}

// validateCapabilities follows the selected model's advertised capabilities.
// It never invents thinking history or silently discards unsupported controls.
func (a *Adaptor) validateCapabilities(request any) error {
	thinking, effort, tools := "", "", false
	var chatRequest *dto.GeneralOpenAIRequest
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		chatRequest = r
		if len(r.ExtraBody) != 0 && string(r.ExtraBody) != "null" && string(r.ExtraBody) != "{}" {
			return errors.New("Send Kimi parameters at the top level instead of extra_body")
		}
		if len(r.EnableThinking) != 0 || len(r.ThinkingBudget) != 0 || len(r.ThinkingTokenBudget) != 0 {
			return errors.New("Use Kimi thinking controls instead of provider-specific thinking parameters")
		}
		if len(r.THINKING) > 0 && string(r.THINKING) != "null" {
			var config struct {
				Type   string `json:"type"`
				Effort string `json:"effort"`
				Budget any    `json:"budget_tokens"`
			}
			if common.Unmarshal(r.THINKING, &config) != nil || (config.Type != "enabled" && config.Type != "disabled") || config.Budget != nil {
				return errors.New("Invalid Kimi thinking controls")
			}
			thinking, effort = config.Type, config.Effort
		}
		if r.ReasoningEffort != "" {
			if effort != "" && effort != r.ReasoningEffort {
				return errors.New("Conflicting Kimi reasoning efforts")
			}
			effort = r.ReasoningEffort
		}
		tools = len(r.Tools) > 0 || len(r.Functions) > 0
		if r.MaxCompletionTokens == nil {
			r.MaxCompletionTokens = r.MaxTokens
		}
		r.MaxTokens = nil
		if r.Stream != nil && *r.Stream {
			if r.StreamOptions == nil {
				r.StreamOptions = &dto.StreamOptions{}
			}
			r.StreamOptions.IncludeUsage = true
		}
	case *dto.ClaudeRequest:
		tools, effort = len(r.GetTools()) > 0, r.GetEfforts()
		if r.Thinking != nil {
			thinking = r.Thinking.Type
			if r.Thinking.BudgetTokens != nil {
				return errors.New("Kimi adaptive thinking does not accept a token budget")
			}
			if thinking == "enabled" {
				r.Thinking.Type = "adaptive"
			}
		}
	case *dto.OpenAIResponsesRequest:
		tools = len(r.Tools) > 0 && string(r.Tools) != "[]" && string(r.Tools) != "null"
		if r.Reasoning != nil {
			effort = r.Reasoning.Effort
			if effort == "none" {
				thinking, effort = "disabled", ""
			}
		}
	}
	if tools && a.metadata.SupportsToolUse != nil && !*a.metadata.SupportsToolUse {
		return errors.New("Kimi model does not support tools")
	}
	if thinking != "" && thinking != "enabled" && thinking != "adaptive" && thinking != "disabled" {
		return errors.New("Invalid Kimi thinking controls")
	}
	if thinking == "disabled" && a.metadata.Reasoning.Type == "only" {
		return errors.New("Kimi model requires thinking")
	}
	if (thinking == "enabled" || thinking == "adaptive" || effort != "") && a.metadata.Reasoning.Type != "both" && a.metadata.Reasoning.Type != "only" {
		return errors.New("Kimi model does not support thinking")
	}
	if effort != "" && (!slices.Contains(a.metadata.Reasoning.Effort.Valid, effort) || thinking == "disabled") {
		return errors.New("Unsupported Kimi reasoning effort")
	}
	if chatRequest != nil && chatRequest.ReasoningEffort != "" {
		config := map[string]any{}
		if len(chatRequest.THINKING) > 0 && string(chatRequest.THINKING) != "null" {
			if err := common.Unmarshal(chatRequest.THINKING, &config); err != nil {
				return err
			}
		}
		if thinking == "" {
			config["type"] = "enabled"
		}
		config["effort"] = effort
		encoded, err := common.Marshal(config)
		if err != nil {
			return err
		}
		chatRequest.THINKING = encoded
		chatRequest.ReasoningEffort = ""
	}
	// Only inspect message content, never tool schemas or arbitrary text.
	if len(a.metadata.Modalities.Input) > 0 {
		raw, err := common.Marshal(request)
		if err != nil {
			return err
		}
		var root map[string]any
		if err := common.Unmarshal(raw, &root); err != nil {
			return err
		}
		for _, field := range []string{"messages", "input"} {
			messages, _ := root[field].([]any)
			for _, message := range messages {
				m, _ := message.(map[string]any)
				content, _ := m["content"].([]any)
				for _, part := range content {
					block, _ := part.(map[string]any)
					typ, _ := block["type"].(string)
					modality := ""
					switch typ {
					case "image", "image_url", "input_image":
						modality = "image"
					case "input_audio", "audio":
						modality = "audio"
					case "video", "video_url", "input_video":
						modality = "video"
					}
					if modality != "" && !slices.Contains(a.metadata.Modalities.Input, modality) {
						return errors.New("Kimi model does not support the requested input modality")
					}
				}
			}
		}
	}
	return nil
}
func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, i *relaycommon.RelayInfo, r *dto.GeneralOpenAIRequest) (any, error) {
	return a.convert(c, i, r)
}
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, i *relaycommon.RelayInfo, r *dto.ClaudeRequest) (any, error) {
	return a.convert(c, i, r)
}
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, i *relaycommon.RelayInfo, r dto.OpenAIResponsesRequest) (any, error) {
	return a.convert(c, i, &r)
}
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if a.credential.Token() == "" {
		return "", errors.New("Kimi credential is not resolved")
	}
	path := "/chat/completions"
	switch a.target {
	case types.RelayFormatClaude:
		path = "/messages?beta=true"
	case types.RelayFormatOpenAIResponses:
		path = "/responses"
	}
	return a.credential.BaseURL() + path, nil
}
func (a *Adaptor) SetupRequestHeader(c *gin.Context, h *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, h)
	kimi.Headers(*h, a.credential)
	h.Set("Content-Type", "application/json")
	if a.target == types.RelayFormatClaude {
		h.Set("anthropic-version", "2023-06-01")
	}
	return nil
}
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (_ any, err error) {
	raw, err := io.ReadAll(io.LimitReader(body, (32<<20)+1))
	if err != nil || len(raw) > 32<<20 {
		return nil, errors.New("Kimi request exceeds size limit")
	}
	for attempt := range 2 {
		endpoint, err := a.GetRequestURL(info)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, endpoint, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		// The request was bounded and buffered above. Keep each retry on an
		// independent reader; the original framework body may already be at EOF.
		req.ContentLength = int64(len(raw))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(raw)), nil
		}
		if err := a.SetupRequestHeader(c, &req.Header, info); err != nil {
			return nil, err
		}
		if err := channel.ApplyHeaderOverride(info, c, req); err != nil {
			return nil, err
		}
		resp, err := channel.DoRequest(c, req, info)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusUnauthorized || attempt == 1 || a.credential.AuthType != "oauth" || c.Writer.Written() {
			return resp, nil
		}
		resp.Body.Close()
		a.credential, err = service.ResolveKimiCredential(c.Request.Context(), info.ChannelId, info.ApiKey, info.ChannelSetting, a.credential.AccessToken)
		if err != nil {
			return nil, err
		}
	}
	return nil, errors.New("Kimi request failed")
}
func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (_ any, apiErr *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewError(errors.New("Invalid Kimi response"), types.ErrorCodeBadResponse)
	}
	info.FinalRequestRelayFormat = a.target
	if info.IsStream {
		resp.Body = newCodingStream(resp.Body, a.target)
	} else {
		b, err := io.ReadAll(io.LimitReader(resp.Body, (32<<20)+1))
		resp.Body.Close()
		if err != nil || len(b) > 32<<20 {
			return nil, types.NewError(errors.New("Invalid Kimi response size"), types.ErrorCodeBadResponseBody)
		}
		if a.target == types.RelayFormatClaude {
			b, err = normalizeUsage(b, false)
			if err != nil {
				return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
		}
		resp.Body = io.NopCloser(bytes.NewReader(b))
	}
	switch a.target {
	case types.RelayFormatClaude:
		return (&claude.Adaptor{}).DoResponse(c, resp, info)
	case types.RelayFormatOpenAIResponses:
		if info.RelayFormat != types.RelayFormatOpenAIResponses {
			if info.IsStream {
				return openai.OaiResponsesToChatStreamHandler(c, info, resp)
			}
			return openai.OaiResponsesToChatHandler(c, info, resp)
		}
		if info.IsStream {
			return openai.OaiResponsesStreamHandler(c, info, resp)
		}
		return openai.OaiResponsesHandler(c, info, resp)
	default:
		if info.RelayFormat == types.RelayFormatOpenAIResponses {
			return nil, types.NewError(errors.New("This Kimi model uses Chat; select a Responses or Anthropic model for Responses requests"), types.ErrorCodeBadResponse)
		}
		if info.IsStream {
			return openai.OaiStreamHandler(c, info, resp)
		}
		return openai.OpenaiHandler(c, info, resp)
	}
}
func (a *Adaptor) GetModelList() []string { return ModelList }
func (a *Adaptor) GetChannelName() string { return ChannelName }
func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("Kimi Gemini requests are not supported")
}
func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, errors.New("Kimi reranking is not supported")
}
func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("Kimi embeddings are not supported")
}
func (a *Adaptor) ConvertAudioRequest(*gin.Context, *relaycommon.RelayInfo, dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("Kimi audio is not supported")
}
func (a *Adaptor) ConvertImageRequest(*gin.Context, *relaycommon.RelayInfo, dto.ImageRequest) (any, error) {
	return nil, errors.New("Kimi image generation is not supported")
}
