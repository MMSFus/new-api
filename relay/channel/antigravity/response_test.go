package antigravity

import (
	"io"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
