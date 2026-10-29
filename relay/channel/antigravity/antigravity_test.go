package antigravity_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	provider "github.com/QuantumNous/new-api/pkg/antigravity"
	"github.com/QuantumNous/new-api/relay"
	channel "github.com/QuantumNous/new-api/relay/channel/antigravity"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNativeChannelRoutingPreservesKimiAndAntigravity(t *testing.T) {
	for _, tc := range []struct {
		channelType int
		apiType     int
		name        string
	}{
		{64, constant.APITypeKimi, "kimi"},
		{65, constant.APITypeAntigravity, "antigravity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			apiType, ok := common.ChannelType2APIType(tc.channelType)
			require.True(t, ok)
			require.Equal(t, tc.apiType, apiType)
			adaptor := relay.GetAdaptor(apiType)
			require.NotNil(t, adaptor)
			assert.Equal(t, tc.name, adaptor.GetChannelName())
		})
	}
}

func TestImportCPAAndSub2API(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		count       int
		expiry      int64
	}{
		{"CPA", `{"type":"antigravity","access_token":"test-access","refresh_token":"test-refresh","expired":"2030-01-01T00:00:00Z","email":"test@example.com","project_id":"test-project","base_url":"https://ignored.invalid"}`, 1, 1893456000},
		{"sub2api API envelope", `{"data":{"accounts":[{"name":"first","platform":"antigravity","type":"oauth","credentials":{"refresh_token":"test-first","expires_at":"1893456000","project_id":"first-project"}},{"platform":"openai","type":"oauth","credentials":{}},{"name":"second","platform":"antigravity","type":"oauth","credentials":{"refresh_token":"test-second"}}],"proxies":[{"url":"https://ignored.invalid"}]}}`, 2, 1893456000},
		{"deduplicate", `[{"refresh_token":"test-refresh"},{"refresh_token":"test-refresh"}]`, 1, 0},
		{"JSON lines", "{\"refresh_token\":\"first\"}\n{\"refresh_token\":\"second\"}", 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accounts, err := provider.Import(tc.input)
			require.NoError(t, err)
			require.Len(t, accounts, tc.count)
			assert.Equal(t, tc.expiry, accounts[0].ExpiresAt)
			assert.NotContains(t, accounts[0].Key(), "ignored.invalid")
			roundtrip, err := provider.Import(accounts[0].Key())
			require.NoError(t, err)
			assert.Equal(t, accounts[0], roundtrip[0])
		})
	}
	for _, raw := range []string{`{}`, `{"type":"codex","access_token":"not-antigravity"}`, `{"platform":"antigravity","type":"apikey","credentials":{"access_token":"bad"}}`, `{"access_token":"expired","expired":"2000-01-01T00:00:00Z"}`, `{"refresh_token":"test","expires_at":{}}`, `{"refresh_token":"bad\nheader"}`, `[{"refresh_token":"valid"},{}]`, `null`} {
		_, err := provider.Import(raw)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "bad\nheader")
	}
	_, err := provider.Import(strings.Repeat(" ", provider.MaxImportBytes+1))
	require.Error(t, err)
	many := strings.Repeat(`{"refresh_token":"test"},`, provider.MaxImportAccounts) + `{"refresh_token":"last"}`
	_, err = provider.Import("[" + many + "]")
	require.Error(t, err)
}

func TestOAuthPKCERefreshAndRedirectProtection(t *testing.T) {
	t.Setenv("ANTIGRAVITY_OAUTH_CLIENT_ID", "test-client")
	t.Setenv("ANTIGRAVITY_OAUTH_CLIENT_SECRET", "test-client-secret")
	verifier, challenge, err := provider.PKCE()
	require.NoError(t, err)
	sum := sha256.Sum256([]byte(verifier))
	assert.Equal(t, base64.RawURLEncoding.EncodeToString(sum[:]), challenge)
	authURL, err := url.Parse(provider.AuthorizationURL("test-state", challenge))
	require.NoError(t, err)
	assert.Equal(t, "S256", authURL.Query().Get("code_challenge_method"))
	assert.Equal(t, "test-state", authURL.Query().Get("state"))
	callback := provider.RedirectURI + "?code=test-code&state=test-state"
	code, state, err := provider.Callback(callback)
	require.NoError(t, err)
	assert.Equal(t, "test-code", code)
	assert.Equal(t, "test-state", state)
	for _, raw := range []string{"test-code", provider.RedirectURI + "?code=x", callback + "&state=other", "https://evil.invalid/callback?code=x&state=y", callback + "#fragment"} {
		_, _, err := provider.Callback(raw)
		require.Error(t, err)
	}
	requests := 0
	client := provider.NewClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		assert.Equal(t, "https://oauth2.googleapis.com/token", req.URL.String())
		require.NoError(t, req.ParseForm())
		assert.Equal(t, provider.OAuthClientID(), req.Form.Get("client_id"))
		if req.Form.Get("grant_type") == "authorization_code" {
			assert.Equal(t, verifier, req.Form.Get("code_verifier"))
			assert.Equal(t, provider.RedirectURI, req.Form.Get("redirect_uri"))
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"access_token":"test-new-access","expires_in":3600}`))}, nil
	})})
	credential, err := provider.Exchange(context.Background(), client, code, verifier)
	require.NoError(t, err)
	assert.NotEmpty(t, credential.AccessToken)
	credential.RefreshToken = "test-preserved-refresh"
	refreshed, err := provider.Refresh(context.Background(), client, credential)
	require.NoError(t, err)
	assert.Equal(t, credential.RefreshToken, refreshed.RefreshToken)
	assert.Equal(t, 2, requests)
	redirected := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected++; w.WriteHeader(200) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer redirect.Close()
	req, err := http.NewRequest("GET", redirect.URL, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer test-never-forward")
	resp, err := provider.NewClient(redirect.Client()).Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 302, resp.StatusCode)
	assert.Zero(t, redirected)
}

const testStream = "data: {\"response\":{\"candidates\":[{\"index\":0,\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"think\",\"thought\":true}]}}]}}\n\n" +
	"data: {\"response\":{\"candidates\":[{\"index\":0,\"content\":{\"parts\":[{\"thoughtSignature\":\"test-original-signature\"}]}}]}}\n\n" +
	"data: {\"response\":{\"candidates\":[{\"index\":0,\"content\":{\"role\":\"model\",\"parts\":[{\"text\":\"answer\"}]},\"finishReason\":\"STOP\"}]}}\n\n" +
	"data: {\"response\":{\"usageMetadata\":{\"promptTokenCount\":11,\"candidatesTokenCount\":7,\"thoughtsTokenCount\":3,\"totalTokenCount\":21,\"cachedContentTokenCount\":4}}}\n\n"

func TestNativeCloudCodeProtocols(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	gin.SetMode(gin.TestMode)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1internal:streamGenerateContent", r.URL.Path)
		assert.Equal(t, "sse", r.URL.Query().Get("alt"))
		assert.Equal(t, "Bearer test-access", r.Header.Get("Authorization"))
		assert.Equal(t, "", r.Header.Get("x-goog-api-key"))
		data, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, "test-project", gjson.GetBytes(data, "project").String())
		assert.Equal(t, "claude-test", gjson.GetBytes(data, "model").String())
		assert.Equal(t, "hello", gjson.GetBytes(data, "request.contents.0.parts.0.text").String())
		assert.Equal(t, int64(0), gjson.GetBytes(data, "request.generationConfig.temperature").Int())
		assert.True(t, gjson.GetBytes(data, "request.generationConfig.temperature").Exists())
		assert.Regexp(t, `^-[0-9]+$`, gjson.GetBytes(data, "request.sessionId").String())
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, testStream)
	}))
	defer server.Close()
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude, types.RelayFormatGemini, types.RelayFormatOpenAIResponses} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", format, stream), func(t *testing.T) {
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
				c.Request.Header.Set("Content-Type", "application/json")
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeAntigravity, ApiType: constant.APITypeAntigravity, ChannelBaseUrl: server.URL, ApiKey: provider.Credential{Type: "antigravity", AccessToken: "test-access", ProjectID: "test-project"}.Key(), UpstreamModelName: "claude-test"}, RelayFormat: format, IsStream: stream, DisablePing: true, StartTime: time.Now()}
				switch format {
				case types.RelayFormatGemini:
					info.RelayMode = relayconstant.RelayModeGemini
				case types.RelayFormatOpenAIResponses:
					info.RelayMode = relayconstant.RelayModeResponses
				}
				a := &channel.Adaptor{}
				a.Init(info)
				var value any
				var err error
				switch format {
				case types.RelayFormatOpenAI:
					var req dto.GeneralOpenAIRequest
					require.NoError(t, common.UnmarshalJsonStr(`{"model":"claude-test","messages":[{"role":"user","content":"hello"}],"temperature":0}`, &req))
					value, err = a.ConvertOpenAIRequest(c, info, &req)
				case types.RelayFormatClaude:
					var req dto.ClaudeRequest
					require.NoError(t, common.UnmarshalJsonStr(`{"model":"claude-test","max_tokens":100,"messages":[{"role":"user","content":"hello"}],"temperature":0}`, &req))
					value, err = a.ConvertClaudeRequest(c, info, &req)
				case types.RelayFormatGemini:
					var req dto.GeminiChatRequest
					require.NoError(t, common.UnmarshalJsonStr(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}],"generationConfig":{"temperature":0}}`, &req))
					value, err = a.ConvertGeminiRequest(c, info, &req)
				case types.RelayFormatOpenAIResponses:
					var req dto.OpenAIResponsesRequest
					require.NoError(t, common.UnmarshalJsonStr(`{"model":"claude-test","input":"hello","temperature":0}`, &req))
					value, err = a.ConvertOpenAIResponsesRequest(c, info, req)
				}
				require.NoError(t, err)
				body, err := common.Marshal(value)
				require.NoError(t, err)
				response, err := a.DoRequest(c, info, strings.NewReader(string(body)))
				require.NoError(t, err)
				resp := response.(*http.Response)
				usage, apiErr := a.DoResponse(c, resp, info)
				require.Nil(t, apiErr)
				require.NotNil(t, usage)
				tokens, ok := usage.(*dto.Usage)
				require.True(t, ok)
				assert.Equal(t, 11, tokens.PromptTokens)
				assert.Equal(t, 10, tokens.CompletionTokens)
				assert.Equal(t, 4, tokens.PromptTokensDetails.CachedTokens)
				assert.Contains(t, recorder.Body.String(), "answer")
				if format == types.RelayFormatClaude || format == types.RelayFormatGemini || format == types.RelayFormatOpenAI {
					assert.Contains(t, recorder.Body.String(), "test-original-signature")
				}
			})
		}
	}
}

func TestRefreshAndProjectDiscoveryAreShared(t *testing.T) {
	var loads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1internal:loadCodeAssist", r.URL.Path)
		loads.Add(1)
		fmt.Fprint(w, `{"cloudaicompanionProject":{"id":"discovered-project"}}`)
	}))
	defer server.Close()
	raw := provider.Credential{Type: "antigravity", AccessToken: "test-shared-project-access"}.Key()
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			c, err := service.ResolveAntigravityCredential(context.Background(), raw, server.URL, dto.ChannelSettings{})
			assert.NoError(t, err)
			assert.Equal(t, "discovered-project", c.ProjectID)
		})
	}
	wg.Wait()
	assert.EqualValues(t, 1, loads.Load())
}

func TestSignedClaudeToolHistoryRoundTrip(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeAntigravity, UpstreamModelName: "claude-test"}, RelayFormat: types.RelayFormatClaude}
	var req dto.ClaudeRequest
	require.NoError(t, common.UnmarshalJsonStr(`{"model":"claude-test","max_tokens":100,"messages":[{"role":"user","content":"lookup"},{"role":"assistant","content":[{"type":"thinking","thinking":"private thought","signature":"test-original-signature"},{"type":"tool_use","id":"call-test","name":"lookup","input":{"q":"test"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-test","content":"found"}]}]}`, &req))
	a := &channel.Adaptor{}
	a.Init(info)
	out, err := a.ConvertClaudeRequest(c, info, &req)
	require.NoError(t, err)
	body, err := common.Marshal(out)
	require.NoError(t, err)
	assert.Equal(t, "test-original-signature", gjson.GetBytes(body, "contents.1.parts.0.thoughtSignature").String())
	assert.Equal(t, "lookup", gjson.GetBytes(body, "contents.1.parts.1.functionCall.name").String())
	assert.Equal(t, "lookup", gjson.GetBytes(body, "contents.2.parts.0.functionResponse.name").String())
	var response dto.GeminiChatResponse
	require.NoError(t, common.UnmarshalJsonStr(`{"candidates":[{"content":{"parts":[{"thought":true,"text":"private thought","thoughtSignature":"test-original-signature"},{"functionCall":{"id":"call-test","name":"lookup","args":{"q":"test"}}}]}}]}`, &response))
	chat := relayconvert.ResponseGeminiChat2OpenAI("test", 1, &response)
	claude := relayconvert.ResponseOpenAI2Claude(chat, info)
	encoded, err := common.Marshal(claude)
	require.NoError(t, err)
	assert.Equal(t, "test-original-signature", gjson.GetBytes(encoded, "content.0.signature").String())
	assert.Equal(t, "tool_use", gjson.GetBytes(encoded, "content.1.type").String())
}

func TestMalformedCloudCodeStreamFails(t *testing.T) {
	for _, data := range []string{"", "data: invalid\n\n", "data: {\"error\":{\"message\":\"secret upstream detail\"}}\n\n"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, data)
		}))
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: provider.Credential{Type: "antigravity", AccessToken: "test-access", ProjectID: "test-project"}.Key()}}
		a := &channel.Adaptor{}
		a.Init(info)
		_, err := a.DoRequest(c, info, strings.NewReader(`{"contents":[]}`))
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "secret upstream detail")
		server.Close()
	}
}

func TestClaudePlainAndThinkingOnlyHistory(t *testing.T) {
	for _, history := range []string{`"prior answer"`, `[{"type":"thinking","thinking":"prior thought","signature":"test-original-signature"}]`} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/messages", nil)
		info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeAntigravity, UpstreamModelName: "claude-test"}, RelayFormat: types.RelayFormatClaude}
		var req dto.ClaudeRequest
		require.NoError(t, common.UnmarshalJsonStr(`{"model":"claude-test","max_tokens":100,"messages":[{"role":"user","content":"first"},{"role":"assistant","content":`+history+`},{"role":"user","content":"next"}]}`, &req))
		a := &channel.Adaptor{}
		a.Init(info)
		value, err := a.ConvertClaudeRequest(c, info, &req)
		require.NoError(t, err)
		data, err := common.Marshal(value)
		require.NoError(t, err)
		assert.Equal(t, "model", gjson.GetBytes(data, "contents.1.role").String())
		assert.NotContains(t, string(data), "SignedThinking")
		assert.NotContains(t, string(data), "context_engineering_is_the_way_to_go")
	}
}

func TestCloudCodeMergesTextDeltasAndSplitsMixedEvents(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			wire := `data: {"response":{"candidates":[{"index":0,"content":{"role":"model","parts":[{"thought":true,"text":"thinking","thoughtSignature":"test-sig"},{"text":"hello"}]}}],"usageMetadata":{"promptTokenCount":11}}}` + "\n\n" +
				`data: {"response":{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":" world"}]},"finishReason":"STOP"}],"usageMetadata":{"candidatesTokenCount":7}}}` + "\n\n"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, wire)
			}))
			defer server.Close()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: provider.Credential{Type: "antigravity", AccessToken: "test-access", ProjectID: "test-project"}.Key()}, IsStream: stream, DisablePing: true}
			a := &channel.Adaptor{}
			a.Init(info)
			value, err := a.DoRequest(c, info, strings.NewReader(`{"contents":[]}`))
			require.NoError(t, err)
			resp := value.(*http.Response)
			defer resp.Body.Close()
			data, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			if stream {
				assert.Equal(t, 3, strings.Count(string(data), "data: "))
				assert.Contains(t, string(data), "test-sig")
			} else {
				assert.Equal(t, "hello world", gjson.GetBytes(data, "candidates.0.content.parts.1.text").String())
				assert.Equal(t, int64(11), gjson.GetBytes(data, "usageMetadata.promptTokenCount").Int())
				assert.Equal(t, int64(7), gjson.GetBytes(data, "usageMetadata.candidatesTokenCount").Int())
			}
		})
	}
}

func TestCanceledRelayDoesNotStartCloudCodeRequest(t *testing.T) {
	var called atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil).WithContext(ctx)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: server.URL, ApiKey: provider.Credential{Type: "antigravity", AccessToken: "test-access", ProjectID: "test-project"}.Key()}}
	a := &channel.Adaptor{}
	a.Init(info)
	_, err := a.DoRequest(c, info, strings.NewReader(`{"contents":[]}`))
	require.Error(t, err)
	assert.Zero(t, called.Load())
}

func TestRejectedAccessTokenRefreshReusesNewCredential(t *testing.T) {
	t.Setenv("ANTIGRAVITY_OAUTH_CLIENT_ID", "test-client")
	t.Setenv("ANTIGRAVITY_OAUTH_CLIENT_SECRET", "test-client-secret")
	previousClient := http.DefaultClient
	var refreshes atomic.Int32
	http.DefaultClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "https://oauth2.googleapis.com/token", req.URL.String())
		n := refreshes.Add(1)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"access_token":"test-retry-%d","expires_in":3600}`, n)))}, nil
	})}
	t.Cleanup(func() { http.DefaultClient = previousClient })
	original := provider.Credential{Type: "antigravity", AccessToken: "test-rejected-once", RefreshToken: "test-retry-refresh", ProjectID: "test-project", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	settings := dto.ChannelSettings{}
	first, err := service.RefreshAntigravityCredential(context.Background(), original.Key(), provider.DefaultBaseURL, settings, original.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, "test-retry-1", first.AccessToken)
	for range 3 {
		next, err := service.RefreshAntigravityCredential(context.Background(), original.Key(), provider.DefaultBaseURL, settings, original.AccessToken)
		require.NoError(t, err)
		assert.Equal(t, first, next)
	}
	assert.EqualValues(t, 1, refreshes.Load())
}

func TestMissingOAuthConfigurationFailsBeforeSendingCredentials(t *testing.T) {
	t.Setenv("ANTIGRAVITY_OAUTH_CLIENT_ID", "")
	t.Setenv("ANTIGRAVITY_OAUTH_CLIENT_SECRET", "")
	called := false
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, fmt.Errorf("unexpected request")
	})}
	_, err := provider.Refresh(context.Background(), client, provider.Credential{RefreshToken: "test-refresh"})
	require.ErrorContains(t, err, "ANTIGRAVITY_OAUTH_CLIENT_ID")
	assert.False(t, called)
}
