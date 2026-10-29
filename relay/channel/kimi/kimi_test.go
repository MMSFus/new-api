package kimi

import (
	"io"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/kimi"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
)

func TestCodingUsagePreservesCacheAndExplicitZero(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		delta     bool
		input     int
		bad       bool
	}{
		{"inclusive message", `{"usage":{"input_tokens":173306,"prompt_tokens":173306,"cache_read_input_tokens":173056,"output_tokens":8}}`, false, 250, false},
		{"exclusive delta", `{"usage":{"input_tokens":250,"prompt_tokens":173306,"cache_read_input_tokens":173056}}`, true, 250, false},
		{"zero delta", `{"usage":{"input_tokens":0,"cache_read_input_tokens":173306}}`, true, 0, false},
		{"message start", `{"message":{"usage":{"input_tokens":10,"cache_read_input_tokens":8,"cache_creation_input_tokens":2}}}`, false, 0, false},
		{"negative subtraction", `{"usage":{"input_tokens":1,"cache_read_input_tokens":2}}`, false, 0, true},
		{"inconsistent totals", `{"usage":{"input_tokens":3,"prompt_tokens":8,"cache_read_input_tokens":2}}`, true, 0, true},
		{"negative output", `{"usage":{"output_tokens":-1}}`, false, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := normalizeUsage([]byte(tc.raw), tc.delta)
			if tc.bad {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			var root map[string]any
			require.NoError(t, common.Unmarshal(raw, &root))
			if message, ok := root["message"].(map[string]any); ok {
				root = message
			}
			usage := root["usage"].(map[string]any)
			require.EqualValues(t, tc.input, usage["input_tokens"])
		})
	}
}

func TestCodingStreamRequiresProtocolCompletion(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		format    types.RelayFormat
		bad       bool
	}{
		{"chat multiline", "data: {\"choices\": [],\ndata: \"usage\":{\"prompt_tokens\":1}}\n\ndata: [DONE]\n\n", types.RelayFormatOpenAI, false},
		{"chat truncated", "data: {\"choices\":[]}\n\n", types.RelayFormatOpenAI, true},
		{"error event", "event: error\ndata: {\"type\":\"error\",\"error\":{\"message\":\"secret\"}}\n\n", types.RelayFormatClaude, true},
		{"claude complete", "data: {\"type\":\"message_delta\",\"usage\":{\"input_tokens\":0}}\n\ndata: {\"type\":\"message_stop\"}\n\n", types.RelayFormatClaude, false},
		{"responses incomplete", "event: response.incomplete\ndata: {\"type\":\"response.incomplete\"}\n\n", types.RelayFormatOpenAIResponses, false},
		{"responses failed", "data: {\"type\":\"response.failed\"}\n\n", types.RelayFormatOpenAIResponses, true},
		{"wrong terminator", "data: [DONE]\n\n", types.RelayFormatClaude, true},
		{"partial event", "data: {\"type\":\"message_stop\"}\n", types.RelayFormatClaude, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := newCodingStream(io.NopCloser(strings.NewReader(tc.raw)), tc.format)
			defer body.Close()
			out, err := io.ReadAll(body)
			if tc.bad {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "secret")
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, out)
		})
	}
}

func TestKimiCapabilitiesKeepZeroAndRejectUnsupportedControls(t *testing.T) {
	a := Adaptor{metadata: kimi.Model{}}
	a.metadata.Reasoning.Type = "both"
	a.metadata.Reasoning.Effort.Valid = []string{"high"}
	alias := &dto.GeneralOpenAIRequest{ReasoningEffort: "high"}
	require.NoError(t, a.validateCapabilities(alias))
	require.JSONEq(t, `{"type":"enabled","effort":"high"}`, string(alias.THINKING))
	require.Empty(t, alias.ReasoningEffort)
	zero := uint(0)
	stream := true
	request := &dto.GeneralOpenAIRequest{MaxTokens: &zero, Stream: &stream}
	require.NoError(t, a.validateCapabilities(request))
	require.NotNil(t, request.MaxCompletionTokens)
	require.EqualValues(t, 0, *request.MaxCompletionTokens)
	require.Nil(t, request.MaxTokens)
	require.True(t, request.StreamOptions.IncludeUsage)
	a.metadata.Reasoning.Type = "only"
	require.ErrorContains(t, a.validateCapabilities(&dto.GeneralOpenAIRequest{THINKING: []byte(`{"type":"disabled"}`)}), "requires thinking")
	require.ErrorContains(t, a.validateCapabilities(&dto.GeneralOpenAIRequest{ReasoningEffort: "low"}), "effort")
	a.metadata.Reasoning.Type = "no"
	require.ErrorContains(t, a.validateCapabilities(&dto.ClaudeRequest{Thinking: &dto.Thinking{Type: "adaptive"}}), "does not support thinking")
	no := false
	a.metadata.SupportsToolUse = &no
	require.ErrorContains(t, a.validateCapabilities(&dto.OpenAIResponsesRequest{Tools: []byte(`[{"type":"function"}]`)}), "tools")
	require.ErrorContains(t, a.validateCapabilities(&dto.GeneralOpenAIRequest{ExtraBody: []byte(`{"max_tokens":-1}`)}), "top level")
	a.metadata.Modalities.Input = []string{"text"}
	var image dto.GeneralOpenAIRequest
	require.NoError(t, common.UnmarshalJsonStr(`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.com/x.png"}}]}]}`, &image))
	require.ErrorContains(t, a.validateCapabilities(&image), "modality")
}
