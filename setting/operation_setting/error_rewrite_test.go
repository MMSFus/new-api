package operation_setting

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseErrorRewriteRulesRejectsInvalidRules(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  string
	}{
		{"malformed json", `{`, "invalid error rewrite rules"},
		{"no condition", `[{"name":"a","response_message":"m"}]`, `#1 "a": at least one of`},
		{"blank keywords are not a condition", `[{"name":"a","keywords":[" "],"response_message":"m"}]`, "at least one of"},
		{"missing message", `[{"name":"a","keywords":["x"],"response_message":" "}]`, "response message is required"},
		{"bad regex", `[{"name":"ok","keywords":["x"],"response_message":"m"},{"name":"b","message_regex":"(","response_message":"m"}]`, `#2 "b": invalid message regex`},
		{"bad status range", `[{"name":"a","status_codes":"599-500","response_message":"m"}]`, "invalid http status code rules"},
		{"bad response status", `[{"name":"a","keywords":["x"],"response_status_code":600,"response_message":"m"}]`, "response status code"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseErrorRewriteRules(tc.value)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}

	rules, err := ParseErrorRewriteRules("")
	require.NoError(t, err)
	assert.Empty(t, rules)
}

func TestMatchErrorRewriteRule(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, ErrorRewriteRulesFromString("[]")) })
	require.NoError(t, ErrorRewriteRulesFromString(`[
		{"name":"disabled","enabled":false,"keywords":["quota"],"response_message":"never"},
		{"name":"channel-only","enabled":true,"channel_ids":[7],"keywords":["quota"],"response_message":"channel"},
		{"name":"all-conditions","enabled":true,"status_codes":"400","error_codes":["upstream_extra_usage_required"],"keywords":["配额策略","third party"],"message_regex":"被其?拒绝","response_status_code":403,"response_error_code":"third_party_client_not_supported","response_message":"当前分组不支持该客户端","skip_retry":true},
		{"name":"fallback","enabled":true,"keywords":["QUOTA"],"response_message":"fallback"}
	]`))

	upstream := func(status int, errType, message string) *types.NewAPIError {
		return types.WithOpenAIError(types.OpenAIError{Message: message, Type: errType}, status)
	}
	extraUsage := "上游对第三方应用调用执行独立的配额策略,本次调用被其拒绝。"
	for _, tc := range []struct {
		name      string
		err       *types.NewAPIError
		channelId int
		want      string
	}{
		{"all conditions match via upstream type", upstream(http.StatusBadRequest, "upstream_extra_usage_required", extraUsage), 1, "all-conditions"},
		{"any keyword matches case-insensitively", types.WithClaudeError(types.ClaudeError{Type: "upstream_extra_usage_required", Message: "Third Party apps 被拒绝"}, http.StatusBadRequest), 1, "all-conditions"},
		{"status mismatch falls through", upstream(http.StatusTooManyRequests, "upstream_extra_usage_required", extraUsage), 1, ""},
		{"regex mismatch falls through", upstream(http.StatusBadRequest, "upstream_extra_usage_required", "配额策略 blocked"), 1, ""},
		{"channel filter matches first", upstream(http.StatusBadRequest, "x", "quota exceeded"), 7, "channel-only"},
		{"channel filter skips other channels", upstream(http.StatusBadRequest, "x", "quota exceeded"), 8, "fallback"},
		{"unmatched local error", types.NewError(errors.New("no match"), types.ErrorCodeGetChannelFailed), 1, ""},
		{"nil error", nil, 1, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := MatchErrorRewriteRule(tc.err, tc.channelId)
			if tc.want == "" {
				assert.Nil(t, rule)
				return
			}
			require.NotNil(t, rule)
			assert.Equal(t, tc.want, rule.Name)
		})
	}

	rule := MatchErrorRewriteRule(upstream(http.StatusBadRequest, "upstream_extra_usage_required", extraUsage), 1)
	require.NotNil(t, rule)
	rewritten := rule.RewriteClientError(types.WithOpenAIError(types.OpenAIError{Message: extraUsage, Type: "upstream_extra_usage_required", Metadata: []byte(`{"raw":"secret"}`)}, http.StatusBadRequest))
	assert.Equal(t, http.StatusForbidden, rewritten.StatusCode)
	assert.True(t, types.IsSkipRetryError(rewritten))
	assert.Empty(t, rewritten.Metadata)
	assert.Equal(t, types.OpenAIError{Message: "当前分组不支持该客户端", Type: "third_party_client_not_supported", Code: "third_party_client_not_supported"}, rewritten.ToOpenAIError())
	assert.Equal(t, types.ClaudeError{Message: "当前分组不支持该客户端", Type: "third_party_client_not_supported"}, rewritten.ToClaudeError())

	fallback := MatchErrorRewriteRule(upstream(http.StatusBadGateway, "x", "quota"), 1)
	require.NotNil(t, fallback)
	kept := fallback.RewriteClientError(upstream(http.StatusBadGateway, "x", "quota"))
	assert.Equal(t, http.StatusBadGateway, kept.StatusCode, "status 0 keeps the original status")
	assert.Equal(t, "upstream_error", kept.ToOpenAIError().Code)
}
