package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/hot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

var simulatedCacheTestChannel = 900000

// useMemorySimulatedCache installs a fresh in-memory prefix store and returns
// a channel id no other test uses.
func useMemorySimulatedCache(t *testing.T) int {
	t.Helper()
	common.RedisEnabled = false
	getSimulatedCacheStore() // consume the Once so it never replaces the test store
	simulatedCacheStore = cachex.NewHybridCache[int](cachex.HybridCacheConfig[int]{
		Namespace: cachex.Namespace(simulatedCacheNamespace),
		Memory: func() *hot.HotCache[string, int] {
			return hot.NewHotCache[string, int](hot.LRU, 1000).WithTTL(simulatedCacheTTL24h).Build()
		},
	})
	simulatedCacheTestChannel++
	return simulatedCacheTestChannel
}

func prepareSimulatedCacheForTest(t *testing.T, channelID int, format types.RelayFormat, hitRate int, body string) *SimulatedCachePlan {
	t.Helper()
	return prepareSimulatedCacheForToken(t, channelID, 1, format, hitRate, body)
}

func prepareSimulatedCacheForToken(t *testing.T, channelID int, tokenID int, format types.RelayFormat, hitRate int, body string) *SimulatedCachePlan {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/test", strings.NewReader(body))
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	info := &relaycommon.RelayInfo{
		TokenId:     tokenID,
		RelayFormat: format,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         channelID,
			UpstreamModelName: "test-model",
			ChannelSetting: dto.ChannelSettings{
				SimulatedCacheEnabled: true,
				SimulatedCacheHitRate: hitRate,
			},
		},
	}
	return PrepareSimulatedCache(c, info)
}

func wrapSimulatedCacheBody(t *testing.T, plan *SimulatedCachePlan, contentType string, body string) string {
	t.Helper()
	resp := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": {contentType}, "Content-Length": {strconv.Itoa(len(body))}},
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
	}
	plan.WrapResponse(resp)
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	if !strings.Contains(contentType, "event-stream") {
		require.Equal(t, int64(len(out)), resp.ContentLength)
		require.Equal(t, strconv.Itoa(len(out)), resp.Header.Get("Content-Length"))
	}
	return string(out)
}

// sseUsagePayloads returns the JSON payload of every data line containing key.
func sseUsagePayloads(body string, key string) []gjson.Result {
	var out []gjson.Result
	for line := range strings.SplitSeq(body, "\n") {
		payload, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "data: ")
		if ok && gjson.Get(payload, key).Exists() {
			out = append(out, gjson.Parse(payload))
		}
	}
	return out
}

func TestSimulatedCacheNumbers(t *testing.T) {
	boundaries := func(lengths []int, ttls ...time.Duration) []simulatedCacheBoundary {
		out := make([]simulatedCacheBoundary, len(lengths))
		for i, l := range lengths {
			out[i] = simulatedCacheBoundary{key: strconv.Itoa(i), length: l}
			if i < len(ttls) {
				out[i].ttl = ttls[i]
			}
		}
		return out
	}
	tests := []struct {
		name   string
		plan   SimulatedCachePlan
		prompt int
		want   simulatedCacheNumbers
	}{
		{
			name:   "openai half prefix floors to 1024+128n",
			plan:   SimulatedCachePlan{clientFormat: types.RelayFormatOpenAI, hitRate: 100, boundaries: boundaries([]int{50, 100}), matched: 0, lastBreakpoint: -1, last1hBreakpoint: -1},
			prompt: 10000,
			want:   simulatedCacheNumbers{prompt: 10000, read: 4992},
		},
		{
			name:   "openai below 1024 is zero",
			plan:   SimulatedCachePlan{clientFormat: types.RelayFormatOpenAI, hitRate: 100, boundaries: boundaries([]int{50, 100}), matched: 0, lastBreakpoint: -1, last1hBreakpoint: -1},
			prompt: 2000,
			want:   simulatedCacheNumbers{prompt: 2000},
		},
		{
			name:   "responses miss is zero",
			plan:   SimulatedCachePlan{clientFormat: types.RelayFormatOpenAIResponses, hitRate: 100, boundaries: boundaries([]int{50, 100}), matched: -1, lastBreakpoint: -1, last1hBreakpoint: -1},
			prompt: 10000,
			want:   simulatedCacheNumbers{prompt: 10000},
		},
		{
			name:   "gemini has threshold but no rounding",
			plan:   SimulatedCachePlan{clientFormat: types.RelayFormatGemini, hitRate: 100, boundaries: boundaries([]int{50, 100}), matched: 0, lastBreakpoint: -1, last1hBreakpoint: -1},
			prompt: 10001,
			want:   simulatedCacheNumbers{prompt: 10001, read: 5000},
		},
		{
			name:   "hit rate caps a full prefix hit",
			plan:   SimulatedCachePlan{clientFormat: types.RelayFormatOpenAI, hitRate: 30, boundaries: boundaries([]int{50, 100}), matched: 1, lastBreakpoint: -1, last1hBreakpoint: -1},
			prompt: 10000,
			want:   simulatedCacheNumbers{prompt: 10000, read: 2944},
		},
		{
			name:   "positive jitter raises the cap",
			plan:   SimulatedCachePlan{clientFormat: types.RelayFormatGemini, hitRate: 30, jitter: 0.03, boundaries: boundaries([]int{50, 100}), matched: 1, lastBreakpoint: -1, last1hBreakpoint: -1},
			prompt: 10000,
			want:   simulatedCacheNumbers{prompt: 10000, read: 3090},
		},
		{
			name: "claude latest turn is plain input and the capped history is created",
			plan: SimulatedCachePlan{claudeClient: true, clientFormat: types.RelayFormatClaude, hitRate: 50,
				boundaries: boundaries([]int{100, 300, 400}, simulatedCacheTTL1h, simulatedCacheTTL5m, simulatedCacheTTL5m), matched: 2, lastBreakpoint: 2, last1hBreakpoint: 0},
			prompt: 4000,
			want:   simulatedCacheNumbers{prompt: 4000, read: 2000, creation: 1000, c5m: 1000},
		},
		{
			name: "claude first turn creates history and bills the latest turn as input",
			plan: SimulatedCachePlan{claudeClient: true, clientFormat: types.RelayFormatClaude, hitRate: 80,
				boundaries: boundaries([]int{100, 300, 400}, simulatedCacheTTL1h, simulatedCacheTTL5m, simulatedCacheTTL5m), matched: -1, lastBreakpoint: 2, last1hBreakpoint: 0},
			prompt: 4000,
			want:   simulatedCacheNumbers{prompt: 4000, creation: 3000, c1h: 1000, c5m: 2000},
		},
		{
			name: "claude miss creates up to last breakpoint split 1h and 5m",
			plan: SimulatedCachePlan{claudeClient: true, clientFormat: types.RelayFormatClaude, hitRate: 100,
				boundaries: boundaries([]int{100, 300, 400}, simulatedCacheTTL1h, simulatedCacheTTL5m), matched: -1, lastBreakpoint: 1, last1hBreakpoint: 0},
			prompt: 4000,
			want:   simulatedCacheNumbers{prompt: 4000, creation: 3000, c1h: 1000, c5m: 2000},
		},
		{
			name: "claude hit on 1h prefix reads it and creates the rest at 5m",
			plan: SimulatedCachePlan{claudeClient: true, clientFormat: types.RelayFormatClaude, hitRate: 100,
				boundaries: boundaries([]int{100, 300, 400}, simulatedCacheTTL1h, simulatedCacheTTL5m), matched: 0, lastBreakpoint: 1, last1hBreakpoint: 0},
			prompt: 4000,
			want:   simulatedCacheNumbers{prompt: 4000, read: 1000, creation: 2000, c5m: 2000},
		},
		{
			name: "claude read is bounded by the last breakpoint",
			plan: SimulatedCachePlan{claudeClient: true, clientFormat: types.RelayFormatClaude, hitRate: 100,
				boundaries: boundaries([]int{100, 300, 400}, simulatedCacheTTL1h, simulatedCacheTTL5m), matched: 2, lastBreakpoint: 1, last1hBreakpoint: 0},
			prompt: 4000,
			want:   simulatedCacheNumbers{prompt: 4000, read: 3000},
		},
		{
			name: "claude hit rate cap turns the rest into creation",
			plan: SimulatedCachePlan{claudeClient: true, clientFormat: types.RelayFormatClaude, hitRate: 25,
				boundaries: boundaries([]int{100, 300, 400}, simulatedCacheTTL1h, simulatedCacheTTL5m), matched: 1, lastBreakpoint: 1, last1hBreakpoint: 0},
			prompt: 4000,
			want:   simulatedCacheNumbers{prompt: 4000, read: 1000, creation: 2000, c5m: 2000},
		},
		{
			name: "claude without cache_control is zero",
			plan: SimulatedCachePlan{claudeClient: true, clientFormat: types.RelayFormatClaude, hitRate: 100,
				boundaries: boundaries([]int{100, 400}), matched: 1, lastBreakpoint: -1, last1hBreakpoint: -1},
			prompt: 4000,
			want:   simulatedCacheNumbers{prompt: 4000},
		},
		{
			name: "claude breakpoint below 1024 tokens is zero",
			plan: SimulatedCachePlan{claudeClient: true, clientFormat: types.RelayFormatClaude, hitRate: 100,
				boundaries: boundaries([]int{100, 300, 400}, simulatedCacheTTL1h, simulatedCacheTTL5m), matched: -1, lastBreakpoint: 1, last1hBreakpoint: 0},
			prompt: 1300,
			want:   simulatedCacheNumbers{prompt: 1300},
		},
		{
			name:   "negative prompt clamps to zero",
			plan:   SimulatedCachePlan{clientFormat: types.RelayFormatOpenAI, hitRate: 100, boundaries: boundaries([]int{50, 100}), matched: 1, lastBreakpoint: -1, last1hBreakpoint: -1},
			prompt: -5,
			want:   simulatedCacheNumbers{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.plan.numbersFor(tt.prompt)
			require.Equal(t, tt.want, got)
			require.Equal(t, got.creation, got.c5m+got.c1h)
			require.GreaterOrEqual(t, got.prompt-got.read-got.creation, 0)
		})
	}
}

func TestSimulatedCacheJitterIsDeterministicAndBounded(t *testing.T) {
	require.Zero(t, simulatedCacheJitterFor(""))
	for i := range 200 {
		id := "req-" + strconv.Itoa(i)
		j := simulatedCacheJitterFor(id)
		require.Equal(t, j, simulatedCacheJitterFor(id))
		require.LessOrEqual(t, j, simulatedCacheJitter)
		require.GreaterOrEqual(t, j, -simulatedCacheJitter)
	}
}

// longText makes a message whose canonical segment is roughly n bytes.
func longText(n int) string {
	return strings.Repeat("a", n)
}

func TestSimulatedCacheOpenAIChatMultiTurn(t *testing.T) {
	channel := useMemorySimulatedCache(t)
	turn1 := `{"model":"m","messages":[{"role":"system","content":"` + longText(8000) + `"},{"role":"user","content":"` + longText(2000) + `"}]}`
	turn2 := `{"model":"m","messages":[{"role":"system","content":"` + longText(8000) + `"},{"role":"user","content":"` + longText(2000) + `"},` +
		`{"role":"assistant","content":"ok"},{"role":"user","content":"` + longText(10) + `"}]}`
	// Real upstream cache numbers in every non-standard field must not survive.
	upstream := `{"id":"x","choices":[{"index":0,"usage":{"cached_tokens":777}}],"timings":{"cache_n":555},` +
		`"usage":{"prompt_tokens":10000,"completion_tokens":20,"total_tokens":10020,"prompt_cache_hit_tokens":999,"prompt_cache_miss_tokens":1,` +
		`"cached_tokens":888,"claude_cache_creation_5_m_tokens":3,"usage_semantic":"anthropic","billing_usage":{"source":"claude_messages","claude_usage":{"input_tokens":1,"cache_read_input_tokens":9000}},` +
		`"prompt_tokens_details":{"cached_tokens":9000,"cache_write_tokens":50,"cached_creation_tokens":50}}}`

	plan := prepareSimulatedCacheForTest(t, channel, types.RelayFormatOpenAI, 100, turn1)
	require.NotNil(t, plan)
	out := gjson.Parse(wrapSimulatedCacheBody(t, plan, "application/json", upstream))
	usage := out.Get("usage")
	assert.Equal(t, int64(0), usage.Get("prompt_tokens_details.cached_tokens").Int())
	assert.Equal(t, int64(0), usage.Get("prompt_cache_hit_tokens").Int())
	assert.Equal(t, int64(10000), usage.Get("prompt_cache_miss_tokens").Int())
	assert.Equal(t, int64(0), usage.Get("cached_tokens").Int())
	assert.Equal(t, int64(0), out.Get("choices.0.usage.cached_tokens").Int())
	assert.Equal(t, int64(10000), usage.Get("prompt_tokens").Int())
	for _, key := range []string{"billing_usage", "usage_semantic", "claude_cache_creation_5_m_tokens",
		"prompt_tokens_details.cache_write_tokens", "prompt_tokens_details.cached_creation_tokens"} {
		assert.False(t, usage.Get(key).Exists(), key)
	}
	assert.False(t, out.Get("timings.cache_n").Exists())

	// Parsed by the real DTO the adaptor uses: billing sees the fake value only.
	var parsed dto.OpenAITextResponse
	require.NoError(t, common.UnmarshalJsonStr(out.Raw, &parsed))
	assert.Nil(t, parsed.Usage.BillingUsage)
	assert.Zero(t, parsed.Usage.PromptTokensDetails.CachedTokens)

	// Turn 2 hits the turn-1 prefix (both messages, ~10000 of ~10050 bytes).
	plan = prepareSimulatedCacheForTest(t, channel, types.RelayFormatOpenAI, 100, turn2)
	require.NotNil(t, plan)
	require.Equal(t, 1, plan.matched)
	out = gjson.Parse(wrapSimulatedCacheBody(t, plan, "application/json", upstream))
	cached := out.Get("usage.prompt_tokens_details.cached_tokens").Int()
	assert.Greater(t, cached, int64(9000))
	assert.Zero(t, (cached-1024)%128)
	assert.Equal(t, cached, out.Get("usage.prompt_cache_hit_tokens").Int())
	assert.Equal(t, 10000-cached, out.Get("usage.prompt_cache_miss_tokens").Int())
	assert.Equal(t, cached, out.Get("choices.0.usage.cached_tokens").Int())

	// Another token sending the same conversation never hits this token's prefixes.
	other := prepareSimulatedCacheForToken(t, channel, 2, types.RelayFormatOpenAI, 100, turn2)
	require.NotNil(t, other)
	require.Equal(t, -1, other.matched)
}

func TestSimulatedCacheStoreBatchOperations(t *testing.T) {
	useMemorySimulatedCache(t)
	store := getSimulatedCacheStore()
	require.NoError(t, store.SetManyWithTTL(map[string]int{"a": 300, "b": 3600}, time.Minute))
	got, err := store.GetMany([]string{"a", "b", "missing"})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"a": 300, "b": 3600}, got)
}

func TestSimulatedCacheOpenAIChatStream(t *testing.T) {
	channel := useMemorySimulatedCache(t)
	req := `{"messages":[{"role":"user","content":"` + longText(9000) + `"}],"stream":true}`
	// Seed the prefix with a first request.
	plan := prepareSimulatedCacheForTest(t, channel, types.RelayFormatOpenAI, 50, req)
	wrapSimulatedCacheBody(t, plan, "text/event-stream", "data: [DONE]\n\n")

	plan = prepareSimulatedCacheForTest(t, channel, types.RelayFormatOpenAI, 50, req)
	require.Equal(t, 0, plan.matched)
	long := `{"choices":[{"delta":{"content":"` + longText(200_000) + `"}}]}`
	upstream := "data: " + long + "\r\n\r\n" +
		": keep-alive\n\n" +
		`data: {"choices":[],"usage":{"prompt_tokens":8000,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":7000}}}` + "\n\n" +
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":8000,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":7000}}}` + "\n\n" +
		"data: [DONE]\n\n"
	out := wrapSimulatedCacheBody(t, plan, "text/event-stream", upstream)

	// Non-usage lines (including a 200K line and CRLF) are byte-identical.
	require.True(t, strings.HasPrefix(out, "data: "+long+"\r\n\r\n: keep-alive\n\n"))
	require.True(t, strings.HasSuffix(out, "data: [DONE]\n\n"))
	frames := sseUsagePayloads(out, "usage")
	require.Len(t, frames, 2)
	for _, frame := range frames {
		// 50% of 8000 with jitter 0 (no request id) = 4000 -> 3968.
		assert.Equal(t, int64(3968), frame.Get("usage.prompt_tokens_details.cached_tokens").Int())
	}
}

func TestSimulatedCacheResponses(t *testing.T) {
	channel := useMemorySimulatedCache(t)
	req := `{"model":"m","instructions":"` + longText(9000) + `","input":"hi","prompt_cache_retention":"24h"}`
	plan := prepareSimulatedCacheForTest(t, channel, types.RelayFormatOpenAIResponses, 100, req)
	require.Equal(t, simulatedCacheTTL24h, plan.writeTTL)
	upstream := `{"id":"r","usage":{"input_tokens":5000,"output_tokens":3,"input_tokens_details":{"cached_tokens":4096},"billing_usage":{"source":"oai_responses"}}}`
	out := gjson.Parse(wrapSimulatedCacheBody(t, plan, "application/json", upstream))
	assert.Equal(t, int64(0), out.Get("usage.input_tokens_details.cached_tokens").Int())
	assert.False(t, out.Get("usage.billing_usage").Exists())

	// Second identical request hits the whole prompt; stored with the 24h tier.
	plan = prepareSimulatedCacheForTest(t, channel, types.RelayFormatOpenAIResponses, 100, req)
	require.Equal(t, len(plan.boundaries)-1, plan.matched)
	require.Equal(t, simulatedCacheTTL24h, plan.matchedTTL)
	stream := "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"usage\":null}}\n\n" +
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":5000,\"output_tokens\":3,\"input_tokens_details\":{\"cached_tokens\":1}}}}\n\n"
	out2 := wrapSimulatedCacheBody(t, plan, "text/event-stream", stream)
	frames := sseUsagePayloads(out2, "response.usage.input_tokens")
	require.Len(t, frames, 1)
	assert.Equal(t, int64(4992), frames[0].Get("response.usage.input_tokens_details.cached_tokens").Int())
	assert.Contains(t, out2, `{"type":"response.created","response":{"usage":null}}`)
}

func TestSimulatedCacheGemini(t *testing.T) {
	channel := useMemorySimulatedCache(t)
	req := `{"systemInstruction":{"parts":[{"text":"` + longText(9000) + `"}]},"contents":[{"role":"user","parts":[{"text":"q"}]}]}`
	plan := prepareSimulatedCacheForTest(t, channel, types.RelayFormatGemini, 100, req)
	wrapSimulatedCacheBody(t, plan, "application/json", `{"candidates":[]}`)

	plan = prepareSimulatedCacheForTest(t, channel, types.RelayFormatGemini, 100, req)
	require.Equal(t, len(plan.boundaries)-1, plan.matched)
	sync := `{"candidates":[],"usageMetadata":{"promptTokenCount":3001,"candidatesTokenCount":4,"cachedContentTokenCount":2,"cacheTokensDetails":[{"modality":"TEXT","tokenCount":2}]}}`
	out := gjson.Parse(wrapSimulatedCacheBody(t, plan, "application/json", sync))
	assert.Equal(t, int64(3001), out.Get("usageMetadata.cachedContentTokenCount").Int())
	assert.False(t, out.Get("usageMetadata.cacheTokensDetails").Exists())

	// Below the 1024 floor the real value is removed, not kept.
	small := `{"usageMetadata":{"promptTokenCount":900,"cachedContentTokenCount":800}}`
	plan = prepareSimulatedCacheForTest(t, channel, types.RelayFormatGemini, 100, req)
	streamOut := wrapSimulatedCacheBody(t, plan, "text/event-stream", "data: "+small+"\n\n")
	frames := sseUsagePayloads(streamOut, "usageMetadata")
	require.Len(t, frames, 1)
	assert.False(t, frames[0].Get("usageMetadata.cachedContentTokenCount").Exists())
}

func claudeRequestForTest(systemTTL string, extraTurn bool) string {
	system := `{"type":"text","text":"` + longText(4000) + `","cache_control":{"type":"ephemeral"` + systemTTL + `}}`
	messages := `{"role":"user","content":[{"type":"text","text":"` + longText(4000) + `","cache_control":{"type":"ephemeral"}}]}`
	if extraTurn {
		messages += `,{"role":"assistant","content":"ok"},{"role":"user","content":"` + longText(2000) + `"}`
	}
	return `{"model":"claude","system":[` + system + `],"messages":[` + messages + `]}`
}

func TestSimulatedCacheClaudeSyncSplit(t *testing.T) {
	channel := useMemorySimulatedCache(t)
	req := claudeRequestForTest(`,"ttl":"1h"`, false)
	upstream := `{"type":"message","usage":{"input_tokens":100,"cache_read_input_tokens":7000,"cache_creation_input_tokens":900,"output_tokens":7,` +
		`"cache_creation":{"ephemeral_5m_input_tokens":900},"claude_cache_creation_1_h_tokens":5,"billing_usage":{"source":"claude_messages"}}}`

	// Turn 1: miss; the system prefix (about half, 1h) is created and the
	// latest user turn is billed as plain input.
	plan := prepareSimulatedCacheForTest(t, channel, types.RelayFormatClaude, 100, req)
	require.Equal(t, -1, plan.matched)
	out := gjson.Parse(wrapSimulatedCacheBody(t, plan, "application/json", upstream))
	usage := out.Get("usage")
	creation := usage.Get("cache_creation_input_tokens").Int()
	assert.InDelta(t, 4000, creation, 50)
	assert.Zero(t, usage.Get("cache_read_input_tokens").Int())
	assert.Equal(t, 8000-creation, usage.Get("input_tokens").Int())
	c1h := usage.Get("cache_creation.ephemeral_1h_input_tokens").Int()
	c5m := usage.Get("cache_creation.ephemeral_5m_input_tokens").Int()
	assert.Equal(t, creation, c1h)
	assert.Equal(t, creation, c1h+c5m)
	assert.Zero(t, c5m)
	assert.False(t, usage.Get("billing_usage").Exists())
	assert.False(t, usage.Get("claude_cache_creation_1_h_tokens").Exists())

	// Turn 2 with a new user turn: reads the whole turn-1 prompt.
	plan = prepareSimulatedCacheForTest(t, channel, types.RelayFormatClaude, 100, claudeRequestForTest(`,"ttl":"1h"`, true))
	require.Equal(t, 1, plan.matched)
	upstream2 := `{"type":"message","usage":{"input_tokens":10000,"cache_read_input_tokens":0,"cache_creation_input_tokens":0,"output_tokens":7}}`
	usage = gjson.Parse(wrapSimulatedCacheBody(t, plan, "application/json", upstream2)).Get("usage")
	read := usage.Get("cache_read_input_tokens").Int()
	assert.InDelta(t, 8000, read, 50)
	assert.Zero(t, usage.Get("cache_creation_input_tokens").Int())
	assert.Equal(t, 10000-read, usage.Get("input_tokens").Int())
	assert.True(t, usage.Get("cache_creation").IsObject())
}

func TestSimulatedCacheClaude5mUpgradedTo1hIsRecreated(t *testing.T) {
	channel := useMemorySimulatedCache(t)
	plan := prepareSimulatedCacheForTest(t, channel, types.RelayFormatClaude, 100, claudeRequestForTest("", false))
	wrapSimulatedCacheBody(t, plan, "application/json", `{"type":"message","usage":{"input_tokens":8000,"output_tokens":1}}`)

	// The system breakpoint now asks for 1h: only cached at 5m, so nothing is
	// read and the system part is re-created at 1h.
	plan = prepareSimulatedCacheForTest(t, channel, types.RelayFormatClaude, 100, claudeRequestForTest(`,"ttl":"1h"`, false))
	require.Equal(t, -1, plan.matched)
	usage := gjson.Parse(wrapSimulatedCacheBody(t, plan, "application/json", `{"type":"message","usage":{"input_tokens":8000,"output_tokens":1}}`)).Get("usage")
	assert.Zero(t, usage.Get("cache_read_input_tokens").Int())
	assert.InDelta(t, 4000, usage.Get("cache_creation.ephemeral_1h_input_tokens").Int(), 50)

	// Third request: the 1h system prefix and the 5m user prefix are both cached.
	plan = prepareSimulatedCacheForTest(t, channel, types.RelayFormatClaude, 100, claudeRequestForTest(`,"ttl":"1h"`, false))
	require.Equal(t, 1, plan.matched)
}

func TestSimulatedCacheClaudeStream(t *testing.T) {
	channel := useMemorySimulatedCache(t)
	req := claudeRequestForTest(`,"ttl":"1h"`, false)
	plan := prepareSimulatedCacheForTest(t, channel, types.RelayFormatClaude, 100, req)
	stream := "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"m","usage":{"input_tokens":3,"cache_read_input_tokens":7997,"cache_creation_input_tokens":0,"output_tokens":1}}}` + "\n\n" +
		"event: content_block_delta\n" + `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"usage"}}` + "\n\n" +
		"event: message_delta\n" + `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":9,"cache_read_input_tokens":7997}}` + "\n\n"
	out := wrapSimulatedCacheBody(t, plan, "text/event-stream", stream)

	start := sseUsagePayloads(out, "message.usage")
	require.Len(t, start, 1)
	usage := start[0].Get("message.usage")
	assert.Zero(t, usage.Get("cache_read_input_tokens").Int())
	creation := usage.Get("cache_creation_input_tokens").Int()
	assert.InDelta(t, 4000, creation, 50)
	assert.Equal(t, 8000-creation, usage.Get("input_tokens").Int())

	delta := sseUsagePayloads(out, "usage.output_tokens")
	require.Len(t, delta, 1)
	dUsage := delta[0].Get("usage")
	assert.False(t, dUsage.Get("input_tokens").Exists())
	assert.Zero(t, dUsage.Get("cache_read_input_tokens").Int())
	assert.Equal(t, creation, dUsage.Get("cache_creation_input_tokens").Int())
	assert.Equal(t, usage.Get("cache_creation").Raw, dUsage.Get("cache_creation").Raw)
	assert.Contains(t, out, `{"type":"content_block_delta","delta":{"type":"text_delta","text":"usage"}}`)
}

func TestSimulatedCacheClaudeWithoutCacheControlIsZero(t *testing.T) {
	channel := useMemorySimulatedCache(t)
	req := `{"model":"claude","system":"` + longText(9000) + `","messages":[{"role":"user","content":"hi"}]}`
	for range 2 {
		plan := prepareSimulatedCacheForTest(t, channel, types.RelayFormatClaude, 100, req)
		usage := gjson.Parse(wrapSimulatedCacheBody(t, plan, "application/json",
			`{"type":"message","usage":{"input_tokens":10,"cache_read_input_tokens":9000,"cache_creation_input_tokens":0,"output_tokens":1}}`)).Get("usage")
		assert.Equal(t, int64(9010), usage.Get("input_tokens").Int())
		assert.Zero(t, usage.Get("cache_read_input_tokens").Int())
		assert.Zero(t, usage.Get("cache_creation_input_tokens").Int())
	}
}

func TestSimulatedCacheClaudeClientOpenAIUpstreamSidecar(t *testing.T) {
	channel := useMemorySimulatedCache(t)
	plan := prepareSimulatedCacheForTest(t, channel, types.RelayFormatClaude, 100, claudeRequestForTest(`,"ttl":"1h"`, false))
	body := wrapSimulatedCacheBody(t, plan, "application/json",
		`{"choices":[],"usage":{"prompt_tokens":8000,"completion_tokens":12,"prompt_tokens_details":{"cached_tokens":5000}}}`)

	var parsed dto.OpenAITextResponse
	require.NoError(t, common.UnmarshalJsonStr(body, &parsed))
	require.NotNil(t, parsed.Usage.BillingUsage)
	canonical, ok := parsed.Usage.BillingUsage.CanonicalUsage()
	require.True(t, ok)
	assert.Equal(t, dto.BillingUsageSemanticAnthropic, canonical.UsageSemantic)
	creation := canonical.PromptTokensDetails.CachedCreationTokens
	assert.InDelta(t, 4000, creation, 50)
	assert.Equal(t, 8000-creation, canonical.PromptTokens)
	assert.Equal(t, 12, canonical.CompletionTokens)
	assert.Equal(t, creation, canonical.ClaudeCacheCreation1hTokens)
	assert.Equal(t, creation, canonical.ClaudeCacheCreation5mTokens+canonical.ClaudeCacheCreation1hTokens)
	assert.Zero(t, parsed.Usage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, creation, parsed.Usage.PromptTokensDetails.CacheCreationTokensTotal())
}

func TestSimulatedCacheWrapResponsePassthrough(t *testing.T) {
	var nilPlan *SimulatedCachePlan
	nilPlan.WrapResponse(&http.Response{})
	plan := &SimulatedCachePlan{matched: -1, lastBreakpoint: -1, last1hBreakpoint: -1,
		boundaries: []simulatedCacheBoundary{{key: "passthrough", length: 1}}, hitRate: 100, writeTTL: simulatedCacheTTL5m}
	useMemorySimulatedCache(t)
	plan.WrapResponse(nil)
	plan.WrapResponse(&http.Response{Header: http.Header{}})

	body := `{"usage":{"prompt_tokens":5000,"prompt_tokens_details":{"cached_tokens":4000}}}`
	for _, header := range []http.Header{
		{"Content-Type": {"application/vnd.amazon.eventstream"}},
		{"Content-Type": {"application/json"}, "Content-Encoding": {"gzip"}},
	} {
		resp := &http.Response{Header: header, Body: io.NopCloser(strings.NewReader(body))}
		plan.WrapResponse(resp)
		out, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, body, string(out))
	}
}
