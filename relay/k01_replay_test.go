package relay

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/k01fixture"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLookupK01ReplayMatchesCompleteRequestForStreamingAndJSON(t *testing.T) {
	streamRequest := map[string]any{
		"model":  "claude-test",
		"stream": true,
		"messages": []any{
			map[string]any{"role": "user", "content": "first"},
		},
	}
	streamResponse := "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	jsonRequest := map[string]any{
		"model":      "claude-test",
		"stream":     false,
		"max_tokens": 64,
		"messages":   []any{map[string]any{"role": "user", "content": "second"}},
	}
	jsonResponse := map[string]any{"id": "fixture-id", "type": "message"}
	fixtureLines := []string{
		string(mustMarshalK01Record(t, map[string]any{"status": 200, "request": streamRequest, "response": streamResponse})),
		string(mustMarshalK01Record(t, map[string]any{"status": 200, "request": jsonRequest, "response": jsonResponse})),
	}
	path := filepath.Join(t.TempDir(), "fixtures.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(fixtureLines[0]+"\n"+fixtureLines[1]+"\n"), 0o600))

	streamBody := []byte(`{"messages":[{"content":"first","role":"user"}],"stream":true,"model":"claude-test"}`)
	streamReplay, matched, _, err := k01fixture.Lookup(path, streamBody)
	require.NoError(t, err)
	require.True(t, matched)
	require.True(t, streamReplay.IsStream)
	require.Equal(t, streamResponse, string(streamReplay.Body))

	jsonBody := []byte(" { \"messages\":[{\"role\":\"user\",\"content\":\"second\"}],\"max_tokens\":64,\"stream\":false,\"model\":\"claude-test\" } ")
	jsonReplay, matched, _, err := k01fixture.Lookup(path, jsonBody)
	require.NoError(t, err)
	require.True(t, matched)
	require.False(t, jsonReplay.IsStream)
	require.JSONEq(t, string(mustMarshalK01Record(t, jsonResponse)), string(jsonReplay.Body))
}

func TestLookupK01ReplayRequiresEveryRequestMemberToMatch(t *testing.T) {
	line := mustMarshalK01Record(t, map[string]any{
		"status": 200,
		"request": map[string]any{
			"model":    "claude-test",
			"stream":   false,
			"messages": []any{map[string]any{"role": "user", "content": "expected"}},
		},
		"response": map[string]any{"type": "message"},
	})
	path := filepath.Join(t.TempDir(), "fixtures.jsonl")
	require.NoError(t, os.WriteFile(path, line, 0o600))

	changedBody := []byte(`{"model":"claude-test","stream":false,"messages":[{"role":"user","content":"different"}]}`)
	_, matched, _, err := k01fixture.Lookup(path, changedBody)
	require.NoError(t, err)
	require.False(t, matched)
}

func TestK01ReplayMatchesEquivalentNumbersAndAcceptsRecordedStatuses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixtures.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("{\"request\":{\"model\":\"m\",\"temperature\":1},\"response\":{\"message\":\"bad\"},\"status\":418}\n{\"request\":{\"model\":\"m\",\"temperature\":9007199254740992},\"response\":{\"message\":\"large\"}}\n"), 0o600))
	for _, number := range []string{"1.0", "1e0"} {
		replay, matched, _, err := k01fixture.Lookup(path, []byte(`{"model":"m","temperature":`+number+`}`))
		require.NoError(t, err)
		require.True(t, matched, number)
		require.Equal(t, 418, replay.Status)
		require.JSONEq(t, `{"message":"bad"}`, string(replay.Body))
	}
	largeReplay, matched, _, err := k01fixture.Lookup(path, []byte(`{"model":"m","temperature":9007199254740992}`))
	require.NoError(t, err)
	require.True(t, matched)
	require.Equal(t, http.StatusOK, largeReplay.Status, "an omitted status defaults to 200")
	_, matched, _, err = k01fixture.Lookup(path, []byte(`{"model":"m","temperature":9007199254740993}`))
	require.NoError(t, err)
	require.False(t, matched, "large integer values must not collapse through float64")

	require.NoError(t, os.WriteFile(path, []byte("{\"request\":{},\"response\":{},\"status\":600}\n"), 0o600))
	_, _, _, err = k01fixture.Lookup(path, []byte(`{}`))
	require.ErrorContains(t, err, "outside 100-599")
}

func TestK01ReplayLoadFailureIsReportedOncePerFileVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixtures.jsonl")
	require.NoError(t, os.WriteFile(path, []byte("not json\n"), 0o600))
	_, _, shouldLog, firstErr := k01fixture.Lookup(path, []byte(`{}`))
	require.Error(t, firstErr)
	require.True(t, shouldLog)
	_, _, shouldLog, secondErr := k01fixture.Lookup(path, []byte(`{}`))
	require.Error(t, secondErr)
	require.False(t, shouldLog)
}

func TestClaudeHelperReturnsNon200ReplayWithoutRetry(t *testing.T) {
	fixturePath := filepath.Join(t.TempDir(), "fixtures.jsonl")
	requestBody := []byte(`{"model":"claude-test","stream":false}`)
	fixture := []byte(`{"status":429,"request":{"model":"claude-test","stream":false},"response":{"error":"limited"}}`)
	require.NoError(t, os.WriteFile(fixturePath, fixture, 0o600))
	t.Setenv(k01fixture.FixtureFileEnv, fixturePath)
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(requestBody))
	common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeAnthropic)
	common.SetContextKey(ctx, constant.ContextKeyChannelSetting, dto.ChannelSettings{Tmp20261106K01Enabled: true})
	common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "claude-test")
	info := &relaycommon.RelayInfo{
		Request:         &dto.ClaudeRequest{Model: "claude-test"},
		RelayFormat:     types.RelayFormatClaude,
		OriginModelName: "claude-test",
	}

	err := ClaudeHelper(ctx, info)
	require.Error(t, err)
	require.Equal(t, 429, recorder.Code)
	require.Equal(t, `{"error":"limited"}`, recorder.Body.String())
	require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	require.Equal(t, "k01", recorder.Header().Get("X-New-Api-Replay"))
	require.True(t, types.IsSkipRetryError(err))
	require.True(t, info.Tmp20261106K01Hit)
}

func mustMarshalK01Record(t *testing.T, value any) []byte {
	t.Helper()
	data, err := common.Marshal(value)
	require.NoError(t, err)
	return data
}

func TestClaudeHelperK01ReplayHitsAndMisses(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Log{}))
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldLogConsume, oldMemoryCache, oldRedis := common.LogConsumeEnabled, common.MemoryCacheEnabled, common.RedisEnabled
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.LogConsumeEnabled, common.MemoryCacheEnabled, common.RedisEnabled = true, false, false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.LogConsumeEnabled, common.MemoryCacheEnabled, common.RedisEnabled = oldLogConsume, oldMemoryCache, oldRedis
	})
	oldStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldStreamingTimeout })
	user := &model.User{Username: "k01-user", Quota: 1_000_000, Group: "default"}
	require.NoError(t, db.Create(user).Error)
	service.InitHttpClient()

	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_upstream","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"live"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":4}}`))
	}))
	t.Cleanup(upstream.Close)

	jsonResponse := `{"id":"msg_fixture","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"replayed"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7}}`
	streamResponse := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_stream\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-test\",\"content\":[],\"usage\":{\"input_tokens\":13,\"output_tokens\":0}}}\n\n" +
		"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
		"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"streamed\"}}\n\n" +
		"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":5}}\n\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	jsonRequest := `{"model":"claude-test","max_tokens":16,"messages":[{"role":"user","content":"json"}]}`
	streamRequest := `{"model":"claude-test","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"stream"}]}`
	missRequest := `{"model":"claude-test","max_tokens":16,"messages":[{"role":"user","content":"miss"}]}`
	fixturePath := filepath.Join(t.TempDir(), "fixtures.jsonl")
	fixture := string(mustMarshalK01Record(t, map[string]any{"request": common.RawMessage(jsonRequest), "response": common.RawMessage(jsonResponse)})) + "\n" +
		string(mustMarshalK01Record(t, map[string]any{"request": common.RawMessage(streamRequest), "response": streamResponse})) + "\n"
	require.NoError(t, os.WriteFile(fixturePath, []byte(fixture), 0o600))
	t.Setenv(k01fixture.FixtureFileEnv, fixturePath)

	for _, tc := range []struct {
		name, body                 string
		stream, replayed           bool
		wantBody                   string
		wantPrompt, wantCompletion int
		wantUpstreamCalls          int32
		brokenFixture              bool
	}{
		{name: "json hit is replayed and billed by fixture usage", body: jsonRequest, replayed: true, wantBody: "replayed", wantPrompt: 11, wantCompletion: 7},
		{name: "stream hit passes fixture events through and bills them", body: streamRequest, stream: true, replayed: true, wantBody: "streamed", wantPrompt: 13, wantCompletion: 5},
		{name: "miss falls through to upstream", body: missRequest, wantBody: "live", wantPrompt: 3, wantCompletion: 4, wantUpstreamCalls: 1},
		{name: "broken fixture falls through to upstream", body: jsonRequest, brokenFixture: true, wantBody: "live", wantPrompt: 3, wantCompletion: 4, wantUpstreamCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, db.Where("1 = 1").Delete(&model.Log{}).Error)
			upstreamCalls.Store(0)
			if tc.brokenFixture {
				brokenPath := filepath.Join(t.TempDir(), "broken.jsonl")
				require.NoError(t, os.WriteFile(brokenPath, []byte("not json\n"), 0o600))
				t.Setenv(k01fixture.FixtureFileEnv, brokenPath)
			}
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(tc.body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeAnthropic)
			common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, upstream.URL)
			common.SetContextKey(ctx, constant.ContextKeyChannelKey, "test-key")
			common.SetContextKey(ctx, constant.ContextKeyChannelSetting, dto.ChannelSettings{Tmp20261106K01Enabled: true})
			common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "claude-test")
			var request dto.ClaudeRequest
			require.NoError(t, common.UnmarshalJsonStr(tc.body, &request))
			info := &relaycommon.RelayInfo{
				Request:         &request,
				RelayFormat:     types.RelayFormatClaude,
				OriginModelName: "claude-test",
				UserId:          user.Id,
				UsingGroup:      "default",
				IsStream:        tc.stream,
			}
			info.PriceData.ModelRatio = 1
			info.PriceData.CompletionRatio = 1
			info.PriceData.GroupRatioInfo.GroupRatio = 1

			newAPIError := ClaudeHelper(ctx, info)
			require.Nil(t, newAPIError)

			assert.Equal(t, tc.wantUpstreamCalls, upstreamCalls.Load())
			assert.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), tc.wantBody)
			assert.Equal(t, tc.replayed, info.Tmp20261106K01Hit)
			if tc.replayed {
				assert.Equal(t, "k01", recorder.Header().Get("X-New-Api-Replay"))
			} else {
				assert.Empty(t, recorder.Header().Get("X-New-Api-Replay"))
			}
			if tc.stream {
				assert.True(t, strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/event-stream"))
				assert.Contains(t, recorder.Body.String(), "message_stop")
			}

			var logs []model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			assert.Equal(t, tc.wantPrompt, logs[0].PromptTokens)
			assert.Equal(t, tc.wantCompletion, logs[0].CompletionTokens)
			assert.Positive(t, logs[0].Quota)
			other, err := common.StrToMap(logs[0].Other)
			require.NoError(t, err)
			if tc.replayed {
				assert.Equal(t, true, other["replay_hit"])
			} else {
				assert.NotContains(t, other, "replay_hit")
			}
		})
	}
}
