package antigravity

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	provider "github.com/QuantumNous/new-api/pkg/antigravity"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Adaptor reuses new-api's protocol converters and Gemini response/accounting
// handlers. Only Cloud Code authentication and wire framing belong here.
type Adaptor struct {
	gemini.Adaptor
	credential provider.Credential
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	if info.ChannelBaseUrl == "" {
		info.ChannelBaseUrl = provider.DefaultBaseURL
	}
	// Cloud Code uses ordinary HTTPS; use the existing no-ALPN HTTP/1 transport.
	info.ChannelSetting.HTTPProtocol = dto.HTTPProtocolHTTP1
}
func (a *Adaptor) GetChannelName() string { return "antigravity" }
func (a *Adaptor) GetModelList() []string { return nil } // Discover the account's actual catalog.
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return strings.TrimRight(info.ChannelBaseUrl, "/") + "/v1internal:streamGenerateContent?alt=sse", nil
}
func (a *Adaptor) SetupRequestHeader(c *gin.Context, h *http.Header, info *relaycommon.RelayInfo) error {
	h.Set("Content-Type", "application/json")
	h.Set("Accept", "text/event-stream")
	h.Set("Authorization", "Bearer "+a.credential.AccessToken)
	h.Set("User-Agent", provider.UserAgent)
	return nil
}
func (a *Adaptor) ConvertEmbeddingRequest(*gin.Context, *relaycommon.RelayInfo, dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("Antigravity does not support embeddings")
}
func (a *Adaptor) ConvertImageRequest(*gin.Context, *relaycommon.RelayInfo, dto.ImageRequest) (any, error) {
	return nil, errors.New("Antigravity image generation is not supported")
}
func (a *Adaptor) ConvertRerankRequest(*gin.Context, int, dto.RerankRequest) (any, error) {
	return nil, errors.New("Antigravity does not support reranking")
}
func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (any, error) {
	if strings.Contains(info.RequestURLPath, ":embedContent") || strings.Contains(info.RequestURLPath, ":batchEmbedContents") {
		return nil, errors.New("Antigravity does not support embeddings")
	}
	credential, err := service.ResolveAntigravityCredential(c.Request.Context(), info.ApiKey, info.ChannelBaseUrl, info.ChannelSetting)
	if err != nil {
		return nil, err
	}
	a.credential = credential
	if err := service.PersistAntigravityCredential(info.ChannelId, info.ApiKey, credential); err != nil {
		return nil, err
	}
	// The host has already applied parameter overrides. Wrap that final payload
	// without moving user overrides to an envelope-specific namespace.
	data, err := io.ReadAll(io.LimitReader(body, 32<<20+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 32<<20 {
		return nil, errors.New("Antigravity request exceeds size limit")
	}
	var payload map[string]any
	if common.Unmarshal(data, &payload) != nil {
		return nil, errors.New("Invalid Antigravity request")
	}
	if payload["sessionId"] == nil {
		// Stable within a conversation and isolated by account credential.
		seed := append([]byte(info.ApiKey), data...)
		if contents, ok := payload["contents"].([]any); ok && len(contents) > 0 {
			first, _ := common.Marshal(contents[0])
			seed = append([]byte(info.ApiKey), first...)
		}
		digest := sha256.Sum256(seed)
		payload["sessionId"] = "-" + strconv.FormatUint(binary.BigEndian.Uint64(digest[:8])&0x7fffffffffffffff, 10)
	}
	framed, err := common.Marshal(map[string]any{"project": credential.ProjectID, "model": info.UpstreamModelName, "requestId": "agent-" + uuid.NewString(), "userAgent": "antigravity", "requestType": "agent", "request": payload})
	if err != nil {
		return nil, err
	}
	resp, err := a.sendCloudCodeRequest(c, info, framed)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized && credential.RefreshToken != "" {
		resp.Body.Close()
		refreshed, refreshErr := service.RefreshAntigravityCredential(c.Request.Context(), info.ApiKey, info.ChannelBaseUrl, info.ChannelSetting, credential.AccessToken)
		if refreshErr != nil {
			return nil, refreshErr
		}
		a.credential = refreshed
		if err := service.PersistAntigravityCredential(info.ChannelId, credential.Key(), refreshed); err != nil {
			return nil, err
		}
		resp, err = a.sendCloudCodeRequest(c, info, framed)
		if err != nil {
			return nil, err
		}
	}
	if resp.StatusCode != http.StatusOK {
		return resp, nil
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		resp.Body.Close()
		return nil, errors.New("Antigravity returned an unexpected response format")
	}
	stream := newResponseStream(resp.Body)
	resp.ContentLength = -1
	resp.Header.Del("Content-Length")
	if info.IsStream {
		resp.Body = stream
		return resp, nil
	}
	data, err = aggregateResponse(stream)
	stream.Close()
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(data))
	resp.Header.Set("Content-Type", "application/json")
	resp.ContentLength = int64(len(data))
	return resp, nil
}

// Signed thinking is validated before using the shared conversion chain.
func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	for _, message := range request.Messages {
		if message.Role != "assistant" || message.IsStringContent() {
			continue
		}
		blocks, err := message.ParseContent()
		if err != nil {
			return nil, err
		}
		for _, block := range blocks {
			if block.Type == "redacted_thinking" {
				return nil, errors.New("Antigravity does not accept redacted thinking history")
			}
			if block.Type == "thinking" && block.Signature == "" {
				return nil, errors.New("Antigravity thinking history requires its original signature")
			}
		}
	}
	value, err := a.Adaptor.ConvertClaudeRequest(c, info, request)
	if converted, ok := value.(*dto.GeminiChatRequest); ok {
		converted.SafetySettings = nil
	}
	return value, err
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	value, err := a.Adaptor.ConvertOpenAIRequest(c, info, request)
	if converted, ok := value.(*dto.GeminiChatRequest); ok {
		converted.SafetySettings = nil
	}
	return value, err
}
func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	value, err := a.Adaptor.ConvertOpenAIResponsesRequest(c, info, request)
	if converted, ok := value.(*dto.GeminiChatRequest); ok {
		converted.SafetySettings = nil
	}
	return value, err
}
func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	value, err := a.Adaptor.ConvertGeminiRequest(c, info, request)
	if converted, ok := value.(*dto.GeminiChatRequest); ok {
		converted.SafetySettings = nil
	}
	return value, err
}

func (a *Adaptor) sendCloudCodeRequest(c *gin.Context, info *relaycommon.RelayInfo, body []byte) (*http.Response, error) {
	endpoint, err := a.GetRequestURL(info)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if err = a.SetupRequestHeader(c, &req.Header, info); err != nil {
		return nil, err
	}
	if err = channel.ApplyHeaderOverride(info, c, req); err != nil {
		return nil, err
	}
	return channel.DoRequest(c, req, info)
}
