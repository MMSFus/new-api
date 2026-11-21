package antigravity

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloudCodeCandidateCompletion(t *testing.T) {
	frame := func(body string) string { return `data: {"response":` + body + "}\n\n" }
	tool := frame(`{"candidates":[{"index":1,"content":{"parts":[{"functionCall":{"name":"lookup","args":{}}}]}}]}`)
	usage := frame(`{"usageMetadata":{"totalTokenCount":12}}`)
	partial := frame(`{"candidates":[{"content":{"parts":[{"text":"partial"}]}}]}`)
	for _, tc := range []struct {
		name, wire, wantError string
		stops                 int
		readError             bool
	}{
		{name: "tool followed by usage", wire: tool + usage, stops: 1},
		{name: "explicit done", wire: tool + usage + "data: [DONE]\n\n", stops: 1},
		{name: "explicit reason", wire: tool + frame(`{"candidates":[{"index":1,"finishReason":"STOP"}]}`) + usage, stops: 1},
		{name: "blocked without candidate", wire: frame(`{"promptFeedback":{"blockReason":"SAFETY"}}`)},
		{name: "partial with usage", wire: partial + usage, wantError: "before candidate completion"},
		{name: "uncompleted sibling", wire: partial + tool + usage, wantError: "before candidate completion"},
		{name: "usage only", wire: usage, wantError: "without candidates"},
		{name: "stale usage", wire: usage + tool, wantError: "before candidate completion"},
		{name: "partial tool", wire: strings.Replace(tool, `"args":{}`, `"args":{},"willContinue":true`, 1) + usage, wantError: "before candidate completion"},
		{name: "trailing error", wire: tool + usage + `{"error":{}}`, wantError: "stream returned an error"},
		{name: "read error", wire: tool + usage, readError: true, wantError: "unexpected EOF"},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", tc.name, stream), func(t *testing.T) {
				var src io.Reader = strings.NewReader(tc.wire)
				if tc.readError {
					src = io.MultiReader(src, iotest.ErrReader(io.ErrUnexpectedEOF))
				}
				s := newResponseStream(io.NopCloser(src))
				defer s.Close()
				var data []byte
				var err error
				if stream {
					data, err = io.ReadAll(s)
				} else {
					data, err = aggregateResponse(s)
				}
				if tc.wantError != "" {
					require.ErrorContains(t, err, tc.wantError)
				} else {
					require.NoError(t, err)
				}
				assert.Equal(t, tc.stops, strings.Count(string(data), `"finishReason":"STOP"`))
				if tc.stops != 0 {
					assert.Contains(t, string(data), `"totalTokenCount":12`)
					assert.Contains(t, string(data), `"name":"lookup"`)
				}
			})
		}
	}
}

func TestAggregateResponseRequiresCandidateTerminalReason(t *testing.T) {
	partial := "data: {\"response\":{\"candidates\":[{\"index\":0,\"content\":{\"parts\":[{\"text\":\"partial answer\"}]}}]}}\n\n"
	stream := newResponseStream(io.NopCloser(strings.NewReader(partial)))
	defer stream.Close()
	_, err := aggregateResponse(stream)
	require.ErrorContains(t, err, "ended before candidate completion")

	for _, reason := range []string{"STOP", "SAFETY"} {
		t.Run(reason, func(t *testing.T) {
			wire := "data: {\"response\":{\"candidates\":[{\"index\":0,\"content\":{\"parts\":[{\"text\":\"complete\"}]},\"finishReason\":\"" + reason + "\"}]}}\n\n"
			stream := newResponseStream(io.NopCloser(strings.NewReader(wire)))
			defer stream.Close()
			data, err := aggregateResponse(stream)
			require.NoError(t, err)
			var result map[string]any
			require.NoError(t, common.Unmarshal(data, &result))
			assert.Equal(t, reason, result["candidates"].([]any)[0].(map[string]any)["finishReason"])
		})
	}
}
