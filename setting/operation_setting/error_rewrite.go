package operation_setting

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
)

const ErrorRewriteRulesOptionKey = "ErrorRewriteRules"

// ErrorRewriteRule replaces the error a client sees when an upstream or relay
// error matches. Non-empty conditions are ANDed; keywords are ORed among
// themselves. The original error is still logged for administrators.
type ErrorRewriteRule struct {
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	ChannelIds []int  `json:"channel_ids,omitempty"`
	// StatusCodes uses the ParseHTTPStatusCodeRanges syntax, e.g. "400,500-599".
	StatusCodes string `json:"status_codes,omitempty"`
	// ErrorCodes matches the error code or the upstream error type exactly.
	ErrorCodes []string `json:"error_codes,omitempty"`
	// Keywords match case-insensitive substrings of the original message.
	Keywords           []string `json:"keywords,omitempty"`
	MessageRegex       string   `json:"message_regex,omitempty"`
	ResponseStatusCode int      `json:"response_status_code,omitempty"`
	ResponseErrorCode  string   `json:"response_error_code,omitempty"`
	ResponseMessage    string   `json:"response_message"`
	SkipRetry          bool     `json:"skip_retry,omitempty"`

	statusRanges  []StatusCodeRange
	lowerKeywords []string
	messageRegex  *regexp.Regexp
}

var errorRewriteRules atomic.Pointer[[]ErrorRewriteRule]

// ParseErrorRewriteRules decodes and validates the option value, returning
// rules ready for matching.
func ParseErrorRewriteRules(value string) ([]ErrorRewriteRule, error) {
	var rules []ErrorRewriteRule
	if strings.TrimSpace(value) == "" {
		return rules, nil
	}
	if err := common.UnmarshalJsonStr(value, &rules); err != nil {
		return nil, fmt.Errorf("invalid error rewrite rules: %w", err)
	}
	for i := range rules {
		rule := &rules[i]
		label := fmt.Sprintf("error rewrite rule #%d %q", i+1, rule.Name)
		ranges, err := ParseHTTPStatusCodeRanges(rule.StatusCodes)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", label, err)
		}
		rule.statusRanges = ranges
		for _, keyword := range rule.Keywords {
			if keyword = strings.TrimSpace(keyword); keyword != "" {
				rule.lowerKeywords = append(rule.lowerKeywords, strings.ToLower(keyword))
			}
		}
		if rule.MessageRegex != "" {
			rule.messageRegex, err = regexp.Compile(rule.MessageRegex)
			if err != nil {
				return nil, fmt.Errorf("%s: invalid message regex: %w", label, err)
			}
		}
		if len(ranges) == 0 && len(rule.ErrorCodes) == 0 && len(rule.lowerKeywords) == 0 && rule.messageRegex == nil {
			return nil, fmt.Errorf("%s: at least one of status codes, error codes, keywords or message regex is required", label)
		}
		if strings.TrimSpace(rule.ResponseMessage) == "" {
			return nil, fmt.Errorf("%s: response message is required", label)
		}
		if rule.ResponseStatusCode != 0 && (rule.ResponseStatusCode < 100 || rule.ResponseStatusCode > 599) {
			return nil, fmt.Errorf("%s: response status code must be 0 or between 100 and 599", label)
		}
	}
	return rules, nil
}

func ErrorRewriteRulesFromString(value string) error {
	rules, err := ParseErrorRewriteRules(value)
	if err != nil {
		return err
	}
	errorRewriteRules.Store(&rules)
	return nil
}

func ErrorRewriteRulesToString() string {
	rules := errorRewriteRules.Load()
	if rules == nil || len(*rules) == 0 {
		return "[]"
	}
	encoded, err := common.Marshal(*rules)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

// MatchErrorRewriteRule returns the first enabled rule matching the original
// error (before the request id is appended), or nil.
func MatchErrorRewriteRule(err *types.NewAPIError, channelId int) *ErrorRewriteRule {
	rules := errorRewriteRules.Load()
	if err == nil || rules == nil {
		return nil
	}
	message := err.Error()
	lowerMessage := strings.ToLower(message)
	codes := []string{string(err.GetErrorCode())}
	switch relayError := err.RelayError.(type) {
	case types.OpenAIError:
		codes = append(codes, relayError.Type)
	case types.ClaudeError:
		codes = append(codes, relayError.Type)
	}
	for i := range *rules {
		rule := &(*rules)[i]
		if !rule.Enabled {
			continue
		}
		if len(rule.ChannelIds) > 0 && !slices.Contains(rule.ChannelIds, channelId) {
			continue
		}
		if len(rule.statusRanges) > 0 && !shouldMatchStatusCodeRanges(rule.statusRanges, err.StatusCode) {
			continue
		}
		if len(rule.ErrorCodes) > 0 && !slices.ContainsFunc(rule.ErrorCodes, func(code string) bool {
			return code != "" && slices.Contains(codes, code)
		}) {
			continue
		}
		if len(rule.lowerKeywords) > 0 && !slices.ContainsFunc(rule.lowerKeywords, func(keyword string) bool {
			return strings.Contains(lowerMessage, keyword)
		}) {
			continue
		}
		if rule.messageRegex != nil && !rule.messageRegex.MatchString(message) {
			continue
		}
		return rule
	}
	return nil
}

// RewriteClientError builds the error shown to the client for a matched rule.
// It carries nothing from the original error except, when the rule keeps it,
// the status code.
func (rule *ErrorRewriteRule) RewriteClientError(original *types.NewAPIError) *types.NewAPIError {
	statusCode := rule.ResponseStatusCode
	if statusCode == 0 {
		statusCode = original.StatusCode
	}
	code := rule.ResponseErrorCode
	if code == "" {
		code = string(types.ErrorTypeUpstreamError)
	}
	return types.WithOpenAIError(types.OpenAIError{Message: rule.ResponseMessage, Type: code, Code: code}, statusCode, types.ErrOptionWithSkipRetry())
}
