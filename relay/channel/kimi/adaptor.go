package kimi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// Adaptor 把 OpenAI chat.completions 请求桥接到 Kimi 网页版会话接口
// （www.kimi.ai / www.kimi.com 的 /api/chat + completion/stream SSE）。
// 每次请求新建一个上游会话，消息历史随请求完整携带。
type Adaptor struct {
	key         *KeyConfig
	hosts       Hosts
	accessToken string
	chatID      string
	// 上游恒为 SSE，框架会按响应 Content-Type 把 info.IsStream 翻成 true，
	// 所以必须记住客户端自己的 stream 意图来决定响应格式。
	clientStream bool
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	key, err := ParseKey(info.ApiKey)
	if err != nil {
		common.SysLog("kimi channel: " + err.Error())
		return
	}
	a.key = key
	a.hosts = key.ResolveHosts(info.ChannelBaseUrl)
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if a.chatID == "" {
		return "", errors.New("kimi channel: chat session not created")
	}
	return fmt.Sprintf("%s/api/chat/%s/completion/stream", a.hosts.Site, a.chatID), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	if a.accessToken == "" {
		return errors.New("kimi channel: access token not resolved")
	}
	req.Set("Authorization", "Bearer "+a.accessToken)
	req.Set("Content-Type", "application/json")
	req.Set("Accept", "text/event-stream")
	req.Set("Origin", a.hosts.Site)
	req.Set("Referer", a.hosts.Site+"/")
	// 覆盖可能由客户端透传的 UA：Cloudflare 按浏览器签名拦截（error 1010）。
	req.Set("User-Agent", browserUA)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("kimi channel: request is nil")
	}
	a.clientStream = request.Stream != nil && *request.Stream
	messages := make([]ChatMessage, 0, len(request.Messages))
	var systemText strings.Builder
	for _, message := range request.Messages {
		content := message.StringContent()
		switch message.Role {
		case "system", "developer":
			systemText.WriteString(content)
			systemText.WriteString("\n")
		case "user", "assistant":
			messages = append(messages, ChatMessage{Role: message.Role, Content: content})
		default:
			return nil, fmt.Errorf("kimi channel: unsupported message role %q", message.Role)
		}
	}
	if systemText.Len() > 0 {
		prefix := strings.TrimSpace(systemText.String())
		if len(messages) > 0 && messages[0].Role == "user" {
			messages[0].Content = prefix + "\n\n" + messages[0].Content
		} else {
			messages = append([]ChatMessage{{Role: "user", Content: prefix}}, messages...)
		}
	}
	if len(messages) == 0 {
		return nil, errors.New("kimi channel: no user/assistant messages")
	}
	return &CompletionRequest{Messages: messages, Refs: []any{}}, nil
}

// createChat 在上游新建会话，返回 chat_id。
func (a *Adaptor) createChat(c *gin.Context, info *relaycommon.RelayInfo, title string) error {
	body, err := common.Marshal(&CreateChatRequest{
		Name:       title,
		BornFrom:   "home",
		KimiplusID: "kimi",
		IsExample:  false,
		Source:     "web",
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost,
		a.hosts.Site+"/api/chat", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", a.hosts.Site)
	req.Header.Set("Referer", a.hosts.Site+"/")
	req.Header.Set("User-Agent", browserUA)

	client, err := service.GetHttpClientWithProxySettings(info.ChannelSetting.Proxy, info.ChannelSetting)
	if err != nil {
		return fmt.Errorf("kimi channel: build http client failed: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("kimi channel: create chat failed: %w", err)
	}
	defer service.CloseResponseBodyGracefully(resp)
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("kimi channel: read create chat response failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("kimi channel: create chat rejected (HTTP %d)", resp.StatusCode)
	}
	var created CreateChatResponse
	if err := common.Unmarshal(respBody, &created); err != nil {
		return fmt.Errorf("kimi channel: invalid create chat response: %w", err)
	}
	if created.ID == "" {
		return errors.New("kimi channel: create chat response missing id")
	}
	a.chatID = created.ID
	return nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if a.key == nil {
		return nil, errors.New("kimi channel: invalid channel key")
	}
	accessToken, err := EnsureAccessToken(info, a.key, a.hosts)
	if err != nil {
		return nil, err
	}
	a.accessToken = accessToken

	title := "new-api"
	if raw, err := io.ReadAll(requestBody); err == nil {
		requestBody = strings.NewReader(string(raw))
		var probe CompletionRequest
		if err := common.Unmarshal(raw, &probe); err == nil {
			for _, m := range probe.Messages {
				if m.Role == "user" && m.Content != "" {
					runes := []rune(strings.TrimSpace(m.Content))
					if len(runes) > 20 {
						runes = runes[:20]
					}
					title = string(runes)
					break
				}
			}
		}
	}
	if err := a.createChat(c, info, title); err != nil {
		return nil, err
	}
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if a.clientStream {
		return kimiStreamHandler(c, info, resp)
	}
	return kimiHandler(c, info, resp)
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("kimi channel: /v1/rerank endpoint not supported")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("kimi channel: /v1/embeddings endpoint not supported")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("kimi channel: audio endpoint not supported")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, errors.New("kimi channel: image endpoint not supported")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("kimi channel: /v1/responses endpoint not supported")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("kimi channel: /v1/messages endpoint not supported")
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("kimi channel: gemini endpoint not supported")
}
