package kimi

import (
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// eventText 提取一个 SSE 事件中应发给客户端的文本增量。
// 只认 cmpl 事件；view=="think" 的增量归入 reasoning_content，其余视图忽略。
func eventText(event StreamEvent) (content string, reasoning string, ok bool) {
	if event.Event != "cmpl" || event.Text == "" {
		return "", "", false
	}
	if event.View == "think" {
		return "", event.Text, true
	}
	if event.View != "" && event.View != "cmpl" {
		return "", "", false
	}
	return event.Text, "", true
}

func streamChunk(info *relaycommon.RelayInfo, content string, reasoning string) *dto.ChatCompletionsStreamResponse {
	response := dto.ChatCompletionsStreamResponse{
		Object:  "chat.completion.chunk",
		Created: common.GetTimestamp(),
		Model:   info.UpstreamModelName,
	}
	var choice dto.ChatCompletionsStreamResponseChoice
	if reasoning != "" {
		choice.Delta.SetReasoningContent(reasoning)
	}
	if content != "" {
		choice.Delta.SetContentString(content)
	}
	response.Choices = append(response.Choices, choice)
	return &response
}

// kimiStreamHandler 把网页版 completion/stream SSE 转换为 OpenAI 流式响应。
// 上游流不携带 token 用量，结束后按估算口径计量（与 dify 等渠道一致）。
func kimiStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	var responseText string
	helper.SetEventStreamHeaders(c)
	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		var event StreamEvent
		if err := common.UnmarshalJsonStr(data, &event); err != nil {
			common.SysLog(common.LogText("kimi channel: error unmarshalling stream event: %s", err.Error()))
			return
		}
		if event.Event == "all_done" {
			sr.Done()
			return
		}
		content, reasoning, ok := eventText(event)
		if !ok {
			return
		}
		responseText += content + reasoning
		if err := helper.ObjectData(c, streamChunk(info, content, reasoning)); err != nil {
			common.SysLog(err.Error())
			sr.Error(err)
		}
	})
	helper.Done(c)
	return service.ResponseText2Usage(c, responseText, info.UpstreamModelName, info.GetEstimatePromptTokens()), nil
}

// kimiHandler 聚合上游 SSE 全文，返回单个 OpenAI 非流式响应。
func kimiHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	service.CloseResponseBodyGracefully(resp)

	var responseText string
	var reasoningText string
	for line := range strings.SplitSeq(string(responseBody), "\n") {
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		var event StreamEvent
		if err := common.UnmarshalJsonStr(strings.TrimSpace(data), &event); err != nil {
			continue
		}
		content, reasoning, ok := eventText(event)
		if !ok {
			continue
		}
		responseText += content
		reasoningText += reasoning
	}

	usage := service.ResponseText2Usage(c, responseText+reasoningText, info.UpstreamModelName, info.GetEstimatePromptTokens())
	fullTextResponse := dto.OpenAITextResponse{
		Id:      "chatcmpl-kimi-" + common.GetRandomString(12),
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
		Model:   info.UpstreamModelName,
	}
	if usage != nil {
		fullTextResponse.Usage = *usage
	}
	message := dto.Message{
		Role:    "assistant",
		Content: responseText,
	}
	if reasoningText != "" {
		message.ReasoningContent = &reasoningText
	}
	fullTextResponse.Choices = append(fullTextResponse.Choices, dto.OpenAITextResponseChoice{
		Index: 0,
		Message: message,
		FinishReason: "stop",
	})
	jsonResponse, err := common.Marshal(fullTextResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, _ = c.Writer.Write(jsonResponse)
	return usage, nil
}
