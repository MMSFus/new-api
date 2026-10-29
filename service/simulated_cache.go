package service

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/samber/hot"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	simulatedCacheNamespace = "new-api:simulated_prompt_cache:v1"
	simulatedCacheCapacity  = 200_000
	// simulatedCacheMaxProbes bounds the longest-prefix lookup per request.
	simulatedCacheMaxProbes = 64
	// simulatedCacheMinTokens is the provider minimum cacheable prefix.
	simulatedCacheMinTokens = 1024
	// simulatedCacheOpenAIStep is OpenAI's cache-hit granularity above the minimum.
	simulatedCacheOpenAIStep = 128

	simulatedCacheTTL5m  = 5 * time.Minute
	simulatedCacheTTL1h  = time.Hour
	simulatedCacheTTL24h = 24 * time.Hour
)

var (
	simulatedCacheStoreOnce sync.Once
	simulatedCacheStore     *cachex.HybridCache[int]
)

// getSimulatedCacheStore returns the prefix store. Values are the TTL tier
// (seconds) the prefix was written with.
func getSimulatedCacheStore() *cachex.HybridCache[int] {
	simulatedCacheStoreOnce.Do(func() {
		if simulatedCacheStore != nil {
			return
		}
		simulatedCacheStore = cachex.NewHybridCache[int](cachex.HybridCacheConfig[int]{
			Namespace: cachex.Namespace(simulatedCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.IntCodec{},
			Memory: func() *hot.HotCache[string, int] {
				return hot.NewHotCache[string, int](hot.LRU, simulatedCacheCapacity).
					WithTTL(simulatedCacheTTL24h).
					WithJanitor().
					Build()
			},
		})
	})
	return simulatedCacheStore
}

// simulatedCacheBoundary is the cumulative state after one request segment.
type simulatedCacheBoundary struct {
	key    string
	length int
	// ttl is the Claude breakpoint tier ending in this segment (0 = none).
	ttl time.Duration
}

// SimulatedCachePlan describes how one relay attempt rewrites upstream
// prompt-cache usage for a channel with simulated caching enabled.
type SimulatedCachePlan struct {
	claudeClient bool
	clientFormat types.RelayFormat
	// targetRate is the cached share of prompt tokens, in percent, drawn
	// from the channel's hit-rate range for this request.
	targetRate float64
	boundaries []simulatedCacheBoundary
	// writeTTL is the non-Claude tier (5m, or 24h for prompt_cache_retention).
	writeTTL time.Duration
	// lastBreakpoint / last1hBreakpoint are boundary indexes (-1 = none).
	lastBreakpoint   int
	last1hBreakpoint int
	// matched is the longest cached boundary index (-1 = miss).
	matched    int
	matchedTTL time.Duration
	// isStream is the client stream flag, used when the upstream
	// Content-Type is neither JSON nor SSE.
	isStream bool
	// numbers are fixed on the first usage frame with a positive prompt so
	// every later frame (message_delta, cumulative Gemini chunks) agrees.
	numbers *simulatedCacheNumbers
	// audit records what upstream reported and what replaced it; the consume
	// log reads it back from the gin context.
	audit *SimulatedCacheAudit
}

// SimulatedCacheTokens is one side (upstream or simulated) of a rewritten
// usage, in the shared cache vocabulary of every relay format.
type SimulatedCacheTokens struct {
	PromptTokens          int `json:"prompt_tokens"`
	CompletionTokens      int `json:"completion_tokens"`
	CacheReadTokens       int `json:"cache_read_tokens"`
	CacheCreationTokens   int `json:"cache_creation_tokens"`
	CacheCreation5mTokens int `json:"cache_creation_5m_tokens"`
	CacheCreation1hTokens int `json:"cache_creation_1h_tokens"`
	Quota                 int `json:"quota"`
}

// SimulatedCacheAudit is persisted under admin_info.simulated_cache of the
// consume log. Fields are only ever added; Version changes only on a breaking
// change, and readers ignore unknown keys.
type SimulatedCacheAudit struct {
	Version int `json:"version"`
	// HitRate is the range maximum, kept for readers of the single rate.
	HitRate    int `json:"hit_rate"`
	HitRateMin int `json:"hit_rate_min"`
	HitRateMax int `json:"hit_rate_max"`
	// TargetRate is the drawn rate in percent; absent on a miss.
	TargetRate      *float64             `json:"target_rate,omitempty"`
	ClientFormat    string               `json:"client_format"`
	UpstreamSchema  string               `json:"upstream_schema"`
	MatchedSegments int                  `json:"matched_segments"`
	TotalSegments   int                  `json:"total_segments"`
	Original        SimulatedCacheTokens `json:"original"`
	Simulated       SimulatedCacheTokens `json:"simulated"`
	QuotaDelta      int                  `json:"quota_delta"`
}

const simulatedCacheAuditVersion = 1

// recordAudit captures the first usage frame of a response: the upstream
// numbers before the rewrite and the simulated numbers after it.
func (p *SimulatedCachePlan) recordAudit(schema string, original SimulatedCacheTokens, n simulatedCacheNumbers) {
	if p.audit == nil || p.audit.UpstreamSchema != "" || n.prompt <= 0 {
		return
	}
	p.audit.UpstreamSchema = schema
	p.audit.Original = original
	p.audit.Simulated = SimulatedCacheTokens{
		PromptTokens:          original.PromptTokens,
		CompletionTokens:      original.CompletionTokens,
		CacheReadTokens:       n.read,
		CacheCreationTokens:   n.creation,
		CacheCreation5mTokens: n.c5m,
		CacheCreation1hTokens: n.c1h,
	}
}

// boundedTokens reads a non-negative token count bounded by MaxQuota.
func boundedTokens(r gjson.Result) int {
	return int(max(0, min(r.Int(), int64(common.MaxQuota))))
}

// simulatedCacheFailedReader replays a read error after the bytes read so far.
type simulatedCacheFailedReader struct{ err error }

func (r simulatedCacheFailedReader) Read([]byte) (int, error) { return 0, r.err }

// simulatedCacheNumbers are the fabricated cache counts for one prompt size.
type simulatedCacheNumbers struct {
	prompt   int
	read     int
	creation int
	c5m      int
	c1h      int
}

// PrepareSimulatedCache builds the plan for the current attempt from the
// client request body. It returns nil when the channel does not enable
// simulated caching or the request carries nothing cacheable.
func PrepareSimulatedCache(c *gin.Context, info *relaycommon.RelayInfo) *SimulatedCachePlan {
	if c == nil {
		return nil
	}
	// Each attempt starts clean so a retry on another channel never logs the
	// previous attempt's audit.
	common.SetContextKey(c, constant.ContextKeySimulatedCache, (*SimulatedCacheAudit)(nil))
	if info == nil || info.ChannelMeta == nil {
		return nil
	}
	setting := info.ChannelSetting
	hitMin, hitMax := setting.SimulatedCacheHitRange()
	if !setting.SimulatedCacheEnabled || hitMax <= 0 {
		return nil
	}
	switch info.RelayFormat {
	case types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, types.RelayFormatClaude, types.RelayFormatGemini:
	default:
		return nil
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil
	}
	body, err := storage.Bytes()
	if err != nil || !gjson.ValidBytes(body) {
		return nil
	}

	plan := &SimulatedCachePlan{
		audit: &SimulatedCacheAudit{
			Version:      simulatedCacheAuditVersion,
			HitRate:      hitMax,
			HitRateMin:   hitMin,
			HitRateMax:   hitMax,
			ClientFormat: string(info.RelayFormat),
		},
		claudeClient:     info.RelayFormat == types.RelayFormatClaude,
		clientFormat:     info.RelayFormat,
		targetRate:       float64(hitMin) + float64(hitMax-hitMin)*simulatedCacheUnitFor(info.RequestId),
		isStream:         info.IsStream,
		writeTTL:         simulatedCacheTTL5m,
		lastBreakpoint:   -1,
		last1hBreakpoint: -1,
		matched:          -1,
	}
	if !plan.claudeClient && gjson.GetBytes(body, "prompt_cache_retention").String() == "24h" {
		plan.writeTTL = simulatedCacheTTL24h
	}

	segments := simulatedCacheSegments(body, info.RelayFormat)
	if len(segments) == 0 {
		return nil
	}
	// Scoped per token so users never hit each other's prefixes.
	prefix := fmt.Sprintf("%d:%d:%s:", info.ChannelId, info.TokenId, info.UpstreamModelName)
	hash := sha256.New()
	var sum []byte
	length := 0
	for i, seg := range segments {
		hash.Reset()
		hash.Write(sum)
		hash.Write(seg.data)
		sum = hash.Sum(nil)
		length += len(seg.data)
		boundary := simulatedCacheBoundary{key: prefix + hex.EncodeToString(sum), length: length}
		if plan.claudeClient && seg.ttl > 0 {
			boundary.ttl = seg.ttl
			plan.lastBreakpoint = i
			if seg.ttl >= simulatedCacheTTL1h {
				plan.last1hBreakpoint = i
			}
		}
		plan.boundaries = append(plan.boundaries, boundary)
	}
	if length == 0 {
		return nil
	}

	plan.lookupLongestPrefix()
	if plan.matched >= 0 {
		rate := math.Round(plan.targetRate*100) / 100
		plan.audit.TargetRate = &rate
	}
	plan.audit.MatchedSegments = plan.matched + 1
	plan.audit.TotalSegments = len(plan.boundaries)
	common.SetContextKey(c, constant.ContextKeySimulatedCache, plan.audit)
	return plan
}

// lookupLongestPrefix sets matched to the longest stored boundary whose stored
// tier satisfies the boundary's breakpoint tier. All candidate keys are read
// in one batch so a long conversation costs a single store round trip.
//
// Anthropic keeps 1h entries separately: a match extending past a 1h
// breakpoint that is only cached at 5m is read only up to before it, and the
// rest is re-created at 1h.
func (p *SimulatedCachePlan) lookupLongestPrefix() {
	last := len(p.boundaries) - 1
	lowest := max(0, last+1-simulatedCacheMaxProbes)
	keys := make([]string, 0, last+1-lowest)
	for k := range p.boundaries {
		if k >= lowest || p.boundaries[k].ttl >= simulatedCacheTTL1h {
			keys = append(keys, p.boundaries[k].key)
		}
	}
	stored, err := getSimulatedCacheStore().GetMany(keys)
	if err != nil {
		common.SysError(common.LogText("simulated cache lookup failed: %s", err.Error()))
		return
	}
	storedTTL := func(k int) (time.Duration, bool) {
		seconds, found := stored[p.boundaries[k].key]
		return time.Duration(seconds) * time.Second, found
	}

	for k := last; k >= lowest; k-- {
		ttl, found := storedTTL(k)
		if !found || ttl < p.boundaries[k].ttl {
			continue
		}
		p.matched = k
		p.matchedTTL = ttl
		break
	}
	for k := 0; k <= p.matched; k++ {
		if p.boundaries[k].ttl < simulatedCacheTTL1h {
			continue
		}
		if ttl, found := storedTTL(k); found && ttl >= simulatedCacheTTL1h {
			continue
		}
		p.matched = -1
		p.matchedTTL = 0
		for j := k - 1; j >= lowest; j-- {
			if ttl, found := storedTTL(j); found && ttl >= p.boundaries[j].ttl {
				p.matched = j
				p.matchedTTL = ttl
				break
			}
		}
		break
	}
}

// simulatedCacheSegment is one canonicalised request segment and the longest
// cache_control ttl it carried (0 = no breakpoint).
type simulatedCacheSegment struct {
	data []byte
	ttl  time.Duration
}

// simulatedCacheSegments splits a client request into the ordered prefix
// segments a provider prompt cache would hash.
func simulatedCacheSegments(body []byte, format types.RelayFormat) []simulatedCacheSegment {
	var raws []gjson.Result
	appendEach := func(r gjson.Result) {
		if r.IsArray() {
			raws = append(raws, r.Array()...)
		} else if r.Exists() && r.Type != gjson.Null {
			raws = append(raws, r)
		}
	}
	root := gjson.ParseBytes(body)
	switch format {
	case types.RelayFormatOpenAI:
		raws = append(raws, root.Get("tools"))
		appendEach(root.Get("messages"))
	case types.RelayFormatOpenAIResponses:
		raws = append(raws, root.Get("tools"), root.Get("instructions"))
		appendEach(root.Get("input"))
	case types.RelayFormatClaude:
		appendEach(root.Get("tools"))
		appendEach(root.Get("system"))
		appendEach(root.Get("messages"))
	case types.RelayFormatGemini:
		raws = append(raws, root.Get("tools"), root.Get("systemInstruction"))
		appendEach(root.Get("contents"))
	}

	segments := make([]simulatedCacheSegment, 0, len(raws))
	for _, raw := range raws {
		if !raw.Exists() || raw.Type == gjson.Null {
			continue
		}
		var value any
		if err := common.UnmarshalJsonStr(raw.Raw, &value); err != nil {
			continue
		}
		value, ttl := stripCacheControl(value)
		data, err := common.Marshal(value)
		if err != nil {
			continue
		}
		segments = append(segments, simulatedCacheSegment{data: data, ttl: ttl})
	}
	// Top-level cache_control (Claude automatic caching) marks the whole prompt.
	if format == types.RelayFormatClaude && len(segments) > 0 {
		if cc := root.Get("cache_control"); cc.IsObject() {
			last := &segments[len(segments)-1]
			last.ttl = max(last.ttl, simulatedCacheBreakpointTTL(cc.Get("ttl").String()))
		}
	}
	return segments
}

// stripCacheControl removes every "cache_control" key recursively so cache
// markers never change a prefix hash, and reports the longest breakpoint ttl.
func stripCacheControl(value any) (any, time.Duration) {
	var ttl time.Duration
	switch v := value.(type) {
	case map[string]any:
		if cc, ok := v["cache_control"]; ok {
			delete(v, "cache_control")
			if ccMap, isMap := cc.(map[string]any); isMap {
				ttlText, _ := ccMap["ttl"].(string)
				ttl = simulatedCacheBreakpointTTL(ttlText)
			}
		}
		for key, child := range v {
			stripped, childTTL := stripCacheControl(child)
			v[key] = stripped
			ttl = max(ttl, childTTL)
		}
	case []any:
		for i, child := range v {
			stripped, childTTL := stripCacheControl(child)
			v[i] = stripped
			ttl = max(ttl, childTTL)
		}
	}
	return value, ttl
}

func simulatedCacheBreakpointTTL(ttl string) time.Duration {
	if ttl == "1h" {
		return simulatedCacheTTL1h
	}
	return simulatedCacheTTL5m
}

// simulatedCacheUnitFor maps a request id to a deterministic value in [0, 1]
// so every frame and retry of one request draws the same rate. A missing id
// takes the middle of the range.
func simulatedCacheUnitFor(requestID string) float64 {
	if requestID == "" {
		return 0.5
	}
	sum := sha256.Sum256([]byte(requestID))
	return float64(binary.BigEndian.Uint64(sum[:8])) / float64(math.MaxUint64)
}

// simulatedCacheTokens truncates a non-negative token estimate into [0, limit].
func simulatedCacheTokens(value float64, limit int) int {
	if limit <= 0 || math.IsNaN(value) || value <= 0 {
		return 0
	}
	if value >= float64(limit) {
		return limit
	}
	return int(value)
}

// prefixTokens is the share of prompt tokens covered by boundary k (-1 = none).
func (p *SimulatedCachePlan) prefixTokens(prompt int, k int) int {
	if k < 0 || k >= len(p.boundaries) {
		return 0
	}
	total := p.boundaries[len(p.boundaries)-1].length
	if total <= 0 {
		return 0
	}
	ratio := float64(p.boundaries[k].length) / float64(total)
	return simulatedCacheTokens(float64(prompt)*ratio, prompt)
}

// numbersFor computes the fabricated cache split for an upstream prompt of
// prompt tokens. It is a pure function of the plan and prompt, so every frame
// of one response agrees.
func (p *SimulatedCachePlan) numbersFor(prompt int) simulatedCacheNumbers {
	prompt = max(0, min(prompt, common.MaxQuota))
	numbers := simulatedCacheNumbers{prompt: prompt}
	if prompt == 0 {
		return numbers
	}
	// A prefix hit reads the drawn share of the prompt, never the latest turn
	// (the last segment); a new conversation (no stored prefix) reads nothing.
	historyTokens := p.prefixTokens(prompt, len(p.boundaries)-2)
	var targetTokens int
	if p.matched >= 0 {
		targetTokens = min(simulatedCacheTokens(float64(prompt)*p.targetRate/100, prompt), historyTokens)
	}

	if !p.claudeClient {
		cached := targetTokens
		if cached < simulatedCacheMinTokens {
			return numbers
		}
		if p.clientFormat == types.RelayFormatOpenAI || p.clientFormat == types.RelayFormatOpenAIResponses {
			cached = simulatedCacheMinTokens + (cached-simulatedCacheMinTokens)/simulatedCacheOpenAIStep*simulatedCacheOpenAIStep
		}
		numbers.read = min(cached, prompt)
		return numbers
	}

	bpTokens := p.prefixTokens(prompt, p.lastBreakpoint)
	if bpTokens < simulatedCacheMinTokens {
		return numbers
	}
	// The latest turn (last message) is billed as plain input; only the span
	// between the cache read and the latest turn, up to the last breakpoint,
	// is written to the cache.
	creationEnd := min(bpTokens, historyTokens)
	numbers.read = min(targetTokens, creationEnd)
	numbers.creation = max(0, creationEnd-numbers.read)
	// Re-created tokens before the last 1h breakpoint are written at 1h.
	oneHourEnd := min(p.prefixTokens(prompt, p.last1hBreakpoint), creationEnd)
	numbers.c1h = min(max(0, oneHourEnd-numbers.read), numbers.creation)
	numbers.c5m = numbers.creation - numbers.c1h
	return numbers
}

// recordPrefixes stores the request prefixes after upstream accepted the
// request, so the next turn of the conversation can hit them. Writes are
// grouped by TTL tier so each tier costs one store round trip.
func (p *SimulatedCachePlan) recordPrefixes() {
	byTTL := make(map[time.Duration]map[string]int)
	write := func(k int, ttl time.Duration) {
		if k == p.matched {
			ttl = max(ttl, p.matchedTTL)
		}
		if byTTL[ttl] == nil {
			byTTL[ttl] = make(map[string]int)
		}
		byTTL[ttl][p.boundaries[k].key] = int(ttl / time.Second)
	}
	if !p.claudeClient {
		write(len(p.boundaries)-1, p.writeTTL)
		if p.matched >= 0 && p.matched != len(p.boundaries)-1 {
			write(p.matched, p.writeTTL)
		}
	} else {
		for k, boundary := range p.boundaries {
			if boundary.ttl > 0 {
				write(k, boundary.ttl)
			}
		}
		// A hit on a prefix this request no longer marks still refreshes it.
		if p.matched >= 0 && p.boundaries[p.matched].ttl == 0 {
			write(p.matched, p.matchedTTL)
		}
	}
	store := getSimulatedCacheStore()
	for ttl, items := range byTTL {
		if err := store.SetManyWithTTL(items, ttl); err != nil {
			common.SysError(common.LogText("simulated cache write failed: %s", err.Error()))
		}
	}
}

// WrapResponse rewrites the usage carried by a successful upstream response,
// streaming or not, before the adaptor parses it. A nil plan is a no-op.
func (p *SimulatedCachePlan) WrapResponse(resp *http.Response) {
	if p == nil || resp == nil || resp.Body == nil {
		return
	}
	p.recordPrefixes()
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "" && !strings.EqualFold(encoding, "identity") {
		return
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	switch {
	case strings.Contains(contentType, "text/event-stream"), contentType == "" && p.isStream:
		resp.Body = &simulatedCacheStreamReader{plan: p, src: bufio.NewReader(resp.Body), body: resp.Body}
		return
	case contentType == "", strings.Contains(contentType, "json"):
	default:
		// AWS eventstream and other binary framings are left untouched.
		return
	}

	data, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		resp.Body = io.NopCloser(io.MultiReader(bytes.NewReader(data), simulatedCacheFailedReader{err: readErr}))
		return
	}
	if gjson.ValidBytes(data) {
		data = p.patchJSON(data)
	} else {
		data = p.patchSSE(data)
	}
	resp.Body = io.NopCloser(bytes.NewReader(data))
	resp.ContentLength = int64(len(data))
	if resp.Header.Get("Content-Length") != "" {
		resp.Header.Set("Content-Length", strconv.Itoa(len(data)))
	}
}

// simulatedCacheStreamReader patches SSE data lines carrying usage. Lines are
// read with bufio.Reader.ReadBytes, which has no line-length limit.
type simulatedCacheStreamReader struct {
	plan    *SimulatedCachePlan
	src     *bufio.Reader
	body    io.Closer
	pending []byte
	err     error
}

func (r *simulatedCacheStreamReader) Read(buf []byte) (int, error) {
	for len(r.pending) == 0 && r.err == nil {
		line, err := r.src.ReadBytes('\n')
		r.err = err
		if len(line) > 0 {
			r.pending = r.plan.patchSSELine(line)
		}
	}
	if len(r.pending) == 0 {
		return 0, r.err
	}
	n := copy(buf, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (r *simulatedCacheStreamReader) Close() error {
	return r.body.Close()
}

// patchSSE patches every data line of a buffered SSE body.
func (p *SimulatedCachePlan) patchSSE(data []byte) []byte {
	if !bytes.Contains(data, []byte("data:")) {
		return data
	}
	out := make([]byte, 0, len(data))
	for len(data) > 0 {
		line := data
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			line = data[:i+1]
		}
		data = data[len(line):]
		out = append(out, p.patchSSELine(line)...)
	}
	return out
}

// patchSSELine patches one SSE line (with its line ending) when it is a data
// line whose JSON payload carries usage; every other line passes through.
func (p *SimulatedCachePlan) patchSSELine(line []byte) []byte {
	content := bytes.TrimRight(line, "\r\n")
	payload, ok := bytes.CutPrefix(content, []byte("data:"))
	if !ok || !bytes.Contains(payload, []byte("usage")) && !bytes.Contains(payload, []byte("Usage")) {
		return line
	}
	payload = bytes.TrimLeft(payload, " ")
	if !gjson.ValidBytes(payload) {
		return line
	}
	patched := p.patchJSON(payload)
	if bytes.Equal(patched, payload) {
		return line
	}
	out := make([]byte, 0, len(line)+len(patched)-len(payload))
	out = append(out, "data: "...)
	out = append(out, patched...)
	return append(out, line[len(content):]...)
}

// patchJSON rewrites every usage object of one JSON document (a sync body or
// one SSE payload). The schema is detected from the usage keys, not from the
// channel type, so any upstream dialect a channel speaks is covered.
func (p *SimulatedCachePlan) patchJSON(doc []byte) []byte {
	root := gjson.ParseBytes(doc)
	if root.IsArray() {
		// Gemini streamGenerateContent without alt=sse returns a JSON array.
		out := doc
		for i := range len(root.Array()) {
			path := strconv.Itoa(i) + ".usageMetadata"
			if usage := gjson.GetBytes(out, path); usage.IsObject() {
				out = p.patchGeminiUsage(out, path, usage)
			}
		}
		return out
	}
	if !root.IsObject() {
		return doc
	}
	docType := root.Get("type").String()
	for _, path := range []string{"usage", "response.usage", "message.usage", "usageMetadata"} {
		usage := gjson.GetBytes(doc, path)
		if !usage.IsObject() {
			continue
		}
		switch {
		case path == "usageMetadata" || usage.Get("promptTokenCount").Exists():
			doc = p.patchGeminiUsage(doc, path, usage)
		case usage.Get("prompt_tokens").Exists():
			doc = p.patchOpenAIChatUsage(doc, path, usage)
		case path == "message.usage" || strings.HasPrefix(docType, "message") ||
			usage.Get("cache_read_input_tokens").Exists() || usage.Get("cache_creation_input_tokens").Exists() ||
			usage.Get("cache_creation").Exists():
			doc = p.patchClaudeUsage(doc, path, usage)
		case usage.Get("input_tokens").Exists():
			doc = p.patchResponsesUsage(doc, path, usage)
		}
	}
	// Moonshot reports cache hits per choice, llama.cpp in timings.cache_n;
	// both are read as fallbacks when cached_tokens is 0.
	read := 0
	if p.numbers != nil {
		read = p.numbers.read
	}
	for i, choice := range gjson.GetBytes(doc, "choices").Array() {
		if choice.Get("usage.cached_tokens").Exists() {
			doc = setJSON(doc, fmt.Sprintf("choices.%d.usage.cached_tokens", i), read)
		}
	}
	return setJSON(doc, "timings.cache_n", nil)
}

// numbersForFrame returns the split for prompt, reusing the first computed
// split so all frames of one response agree.
func (p *SimulatedCachePlan) numbersForFrame(prompt int) simulatedCacheNumbers {
	if p.numbers != nil && p.numbers.prompt == prompt {
		return *p.numbers
	}
	numbers := p.numbersFor(prompt)
	if prompt > 0 && p.numbers == nil {
		p.numbers = &numbers
	}
	return numbers
}

// setJSON sets (or, for a nil value, deletes) path, keeping the document
// unchanged when the edit fails.
func setJSON(doc []byte, path string, value any) []byte {
	if value == nil {
		if !gjson.GetBytes(doc, path).Exists() {
			return doc
		}
		out, err := sjson.DeleteBytes(doc, path)
		if err != nil {
			return doc
		}
		return out
	}
	raw, err := common.Marshal(value)
	if err != nil {
		return doc
	}
	out, err := sjson.SetRawBytes(doc, path, raw)
	if err != nil {
		return doc
	}
	return out
}

// stripUpstreamUsageSidecar removes keys through which a chained new-api
// upstream (or Claude-derived OpenAI usage) would leak real cache numbers
// into billing, which prefers the sidecar over the top-level fields.
func stripUpstreamUsageSidecar(doc []byte, path string) []byte {
	for _, key := range []string{"billing_usage", "usage_semantic", "usage_source",
		"claude_cache_creation_5_m_tokens", "claude_cache_creation_1_h_tokens"} {
		doc = setJSON(doc, path+"."+key, nil)
	}
	return doc
}

// claudeSidecar is the Claude billing snapshot injected when a Claude client
// is served by a non-Claude upstream schema, the only carrier of cache
// creation and its 5m/1h split into billing and the client conversion.
func (n simulatedCacheNumbers) claudeSidecar(outputTokens int) *dto.BillingUsage {
	return &dto.BillingUsage{
		Source:   dto.BillingUsageSourceClaudeMessages,
		Semantic: dto.BillingUsageSemanticAnthropic,
		ClaudeUsage: &dto.ClaudeUsage{
			InputTokens:              n.prompt - n.read - n.creation,
			CacheReadInputTokens:     n.read,
			CacheCreationInputTokens: n.creation,
			OutputTokens:             max(0, outputTokens),
			CacheCreation: &dto.ClaudeCacheCreationUsage{
				Ephemeral5mInputTokens: n.c5m,
				Ephemeral1hInputTokens: n.c1h,
			},
		},
	}
}

// zeroToNil turns a zero count into a delete so absent cache-write fields
// stay absent.
func zeroToNil(v int) any {
	if v == 0 {
		return nil
	}
	return v
}

func (p *SimulatedCachePlan) patchOpenAIChatUsage(doc []byte, path string, usage gjson.Result) []byte {
	doc = stripUpstreamUsageSidecar(doc, path)
	prompt := usage.Get("prompt_tokens").Int()
	if prompt <= 0 {
		return doc
	}
	n := p.numbersForFrame(int(min(prompt, int64(common.MaxQuota))))
	originalRead := boundedTokens(usage.Get("prompt_tokens_details.cached_tokens"))
	if originalRead == 0 {
		originalRead = max(boundedTokens(usage.Get("prompt_cache_hit_tokens")), boundedTokens(usage.Get("cached_tokens")),
			boundedTokens(usage.Get("input_tokens_details.cached_tokens")))
	}
	p.recordAudit("openai_chat", SimulatedCacheTokens{
		PromptTokens:     n.prompt,
		CompletionTokens: boundedTokens(usage.Get("completion_tokens")),
		CacheReadTokens:  originalRead,
		CacheCreationTokens: max(boundedTokens(usage.Get("prompt_tokens_details.cache_write_tokens")),
			boundedTokens(usage.Get("prompt_tokens_details.cached_creation_tokens"))),
	}, n)
	doc = setJSON(doc, path+".prompt_tokens_details.cached_tokens", n.read)
	doc = setJSON(doc, path+".prompt_tokens_details.cached_creation_tokens", zeroToNil(n.creation))
	doc = setJSON(doc, path+".prompt_tokens_details.cache_write_tokens", zeroToNil(n.creation))
	if usage.Get("prompt_cache_hit_tokens").Exists() {
		doc = setJSON(doc, path+".prompt_cache_hit_tokens", n.read)
	}
	if usage.Get("prompt_cache_miss_tokens").Exists() {
		doc = setJSON(doc, path+".prompt_cache_miss_tokens", n.prompt-n.read)
	}
	if usage.Get("cached_tokens").Exists() {
		doc = setJSON(doc, path+".cached_tokens", n.read)
	}
	if usage.Get("input_tokens_details").IsObject() {
		doc = setJSON(doc, path+".input_tokens_details.cached_tokens", n.read)
		doc = setJSON(doc, path+".input_tokens_details.cached_creation_tokens", zeroToNil(n.creation))
		doc = setJSON(doc, path+".input_tokens_details.cache_write_tokens", zeroToNil(n.creation))
	}
	if p.claudeClient && n.creation > 0 {
		doc = setJSON(doc, path+".billing_usage", n.claudeSidecar(int(usage.Get("completion_tokens").Int())))
	}
	return doc
}

func (p *SimulatedCachePlan) patchResponsesUsage(doc []byte, path string, usage gjson.Result) []byte {
	doc = stripUpstreamUsageSidecar(doc, path)
	prompt := usage.Get("input_tokens").Int()
	if prompt <= 0 {
		return doc
	}
	n := p.numbersForFrame(int(min(prompt, int64(common.MaxQuota))))
	p.recordAudit("openai_responses", SimulatedCacheTokens{
		PromptTokens:     n.prompt,
		CompletionTokens: boundedTokens(usage.Get("output_tokens")),
		CacheReadTokens:  boundedTokens(usage.Get("input_tokens_details.cached_tokens")),
		CacheCreationTokens: max(boundedTokens(usage.Get("input_tokens_details.cache_write_tokens")),
			boundedTokens(usage.Get("input_tokens_details.cached_creation_tokens"))),
	}, n)
	doc = setJSON(doc, path+".input_tokens_details.cached_tokens", n.read)
	doc = setJSON(doc, path+".input_tokens_details.cached_creation_tokens", zeroToNil(n.creation))
	doc = setJSON(doc, path+".input_tokens_details.cache_write_tokens", zeroToNil(n.creation))
	if p.claudeClient && n.creation > 0 {
		doc = setJSON(doc, path+".billing_usage", n.claudeSidecar(int(usage.Get("output_tokens").Int())))
	}
	return doc
}

func (p *SimulatedCachePlan) patchGeminiUsage(doc []byte, path string, usage gjson.Result) []byte {
	prompt := usage.Get("promptTokenCount").Int()
	doc = setJSON(doc, path+".billing_usage", nil)
	doc = setJSON(doc, path+".cacheTokensDetails", nil)
	if prompt <= 0 {
		return setJSON(doc, path+".cachedContentTokenCount", nil)
	}
	n := p.numbersForFrame(int(min(prompt, int64(common.MaxQuota))))
	p.recordAudit("gemini", SimulatedCacheTokens{
		PromptTokens:     n.prompt,
		CompletionTokens: min(boundedTokens(usage.Get("candidatesTokenCount"))+boundedTokens(usage.Get("thoughtsTokenCount")), common.MaxQuota),
		CacheReadTokens:  boundedTokens(usage.Get("cachedContentTokenCount")),
	}, n)
	doc = setJSON(doc, path+".cachedContentTokenCount", zeroToNil(n.read))
	if p.claudeClient && n.creation > 0 {
		output := usage.Get("candidatesTokenCount").Int() + usage.Get("thoughtsTokenCount").Int()
		doc = setJSON(doc, path+".billing_usage", n.claudeSidecar(int(min(output, int64(common.MaxQuota)))))
	}
	return doc
}

func (p *SimulatedCachePlan) patchClaudeUsage(doc []byte, path string, usage gjson.Result) []byte {
	doc = stripUpstreamUsageSidecar(doc, path)
	input := usage.Get("input_tokens")
	var n simulatedCacheNumbers
	switch {
	case input.Exists():
		// Each part is clamped before summing so huge upstream values cannot overflow.
		var prompt int64
		for _, key := range []string{"input_tokens", "cache_read_input_tokens", "cache_creation_input_tokens"} {
			prompt += max(0, min(usage.Get(key).Int(), int64(common.MaxQuota)))
		}
		n = p.numbersForFrame(int(min(prompt, int64(common.MaxQuota))))
		original := SimulatedCacheTokens{
			PromptTokens:          n.prompt,
			CompletionTokens:      boundedTokens(usage.Get("output_tokens")),
			CacheReadTokens:       boundedTokens(usage.Get("cache_read_input_tokens")),
			CacheCreationTokens:   boundedTokens(usage.Get("cache_creation_input_tokens")),
			CacheCreation5mTokens: boundedTokens(usage.Get("cache_creation.ephemeral_5m_input_tokens")),
			CacheCreation1hTokens: boundedTokens(usage.Get("cache_creation.ephemeral_1h_input_tokens")),
		}
		p.recordAudit("claude", original, n)
		doc = setJSON(doc, path+".input_tokens", n.prompt-n.read-n.creation)
	case p.numbers != nil:
		// message_delta without input_tokens: keep the message_start split.
		n = *p.numbers
	}
	hasCache := usage.Get("cache_read_input_tokens").Exists() || usage.Get("cache_creation_input_tokens").Exists() ||
		usage.Get("cache_creation").Exists()
	if !input.Exists() && !hasCache {
		return doc
	}
	doc = setJSON(doc, path+".cache_read_input_tokens", n.read)
	doc = setJSON(doc, path+".cache_creation_input_tokens", n.creation)
	// Always write the object: billing falls back to flat legacy fields when
	// cache_creation is absent.
	return setJSON(doc, path+".cache_creation", map[string]int{
		"ephemeral_5m_input_tokens": n.c5m,
		"ephemeral_1h_input_tokens": n.c1h,
	})
}

// originalUsageForAudit rebuilds the settled usage with upstream's real cache
// numbers, so the audit can price the request as if nothing was rewritten.
func originalUsageForAudit(billingUsage *dto.Usage, original SimulatedCacheTokens, anthropic bool) *dto.Usage {
	usage := *billingUsage
	usage.BillingUsage = nil
	usage.PromptTokensDetails.CachedTokens = original.CacheReadTokens
	usage.PromptTokensDetails.CachedCreationTokens = original.CacheCreationTokens
	usage.PromptTokensDetails.CacheWriteTokens = 0
	usage.ClaudeCacheCreation5mTokens = 0
	usage.ClaudeCacheCreation1hTokens = 0
	if anthropic {
		// Claude semantic prompt tokens exclude both cache parts.
		usage.PromptTokens = max(0, original.PromptTokens-original.CacheReadTokens-original.CacheCreationTokens)
		usage.ClaudeCacheCreation5mTokens = original.CacheCreation5mTokens
		usage.ClaudeCacheCreation1hTokens = original.CacheCreation1hTokens
	}
	return &usage
}

// attachSimulatedCacheAudit records, for administrators, the upstream usage
// that the simulated cache replaced and what that usage would have cost.
// Pricing is re-run without side effects on the request's billing state.
func attachSimulatedCacheAudit(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, billingUsage *dto.Usage,
	summary textQuotaSummary, other *model.LogOther) {
	audit, ok := common.GetContextKeyType[*SimulatedCacheAudit](ctx, constant.ContextKeySimulatedCache)
	if !ok || audit == nil || audit.UpstreamSchema == "" || billingUsage == nil || other == nil {
		return
	}
	record := *audit
	record.Original.CompletionTokens = summary.CompletionTokens
	record.Simulated.CompletionTokens = summary.CompletionTokens
	record.Simulated.Quota = summary.Quota

	savedClamp := relayInfo.QuotaClamp
	originalUsage := originalUsageForAudit(billingUsage, record.Original, summary.IsClaudeUsageSemantic)
	originalSummary := calculateTextQuotaSummary(ctx, relayInfo, originalUsage)
	record.Original.Quota = settleTextQuota(relayInfo, &originalSummary, originalUsage)
	relayInfo.QuotaClamp = savedClamp

	record.QuotaDelta = record.Simulated.Quota - record.Original.Quota
	other.SetAdmin("simulated_cache", record)
	// The upstream bills the real usage, not the simulated one.
	other.SetCostBasisQuota(record.Original.Quota)
}
