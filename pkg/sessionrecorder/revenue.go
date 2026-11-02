package sessionrecorder

import (
	"math"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

const (
	// RevenueRulesOptionKey holds the revenue rules as a JSON array.
	RevenueRulesOptionKey = "session_recorder_revenue.rules"
	// RevenueCurrencyLabelOptionKey is a display-only currency label.
	RevenueCurrencyLabelOptionKey = "session_recorder_revenue.currency_label"

	MaxRevenueRules            = 100
	maxRevenueRuleIDLen        = 64
	maxRevenueRuleNameLen      = 100
	maxRevenuePatternLen       = 512
	MaxRevenueCurrencyLabelLen = 16

	RevenueMatchKeyword = "keyword"
	RevenueMatchRegex   = "regex"
	RevenueMatchGlob    = "glob"
)

// RevenueRule prices one class of models per TB (10^12 bytes) of compressed
// archive data. Rules are ordered; the first enabled rule that matches the
// model name applies.
type RevenueRule struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	MatchType  string  `json:"match_type"`
	Pattern    string  `json:"pattern"`
	PricePerTB float64 `json:"price_per_tb"`
	Enabled    bool    `json:"enabled"`
}

type compiledRevenueRule struct {
	rule    RevenueRule
	keyword string
	regex   *regexp.Regexp
	glob    modelMatcher
}

func (r compiledRevenueRule) match(model string) bool {
	switch r.rule.MatchType {
	case RevenueMatchKeyword:
		return strings.Contains(strings.ToLower(model), r.keyword)
	case RevenueMatchRegex:
		return r.regex.MatchString(model)
	default:
		return r.glob.match(model)
	}
}

// RevenueMatcher is a validated, compiled rule list.
type RevenueMatcher struct {
	rules []compiledRevenueRule
}

// CompileRevenueRules validates every rule, enabled or not, and compiles the
// enabled ones. Errors are web console messages.
func CompileRevenueRules(rules []RevenueRule) (*RevenueMatcher, error) {
	if len(rules) > MaxRevenueRules {
		return nil, common.NewMessage("At most {{max}} revenue rules are allowed", map[string]any{"max": MaxRevenueRules})
	}
	seen := make(map[string]struct{}, len(rules))
	matcher := &RevenueMatcher{}
	for i, rule := range rules {
		position := map[string]any{"index": i + 1}
		id := strings.TrimSpace(rule.ID)
		if id == "" || utf8.RuneCountInString(id) > maxRevenueRuleIDLen {
			return nil, common.NewMessage("Revenue rule {{index}} has an invalid ID", position)
		}
		if _, dup := seen[id]; dup {
			return nil, common.NewMessage("Revenue rule {{index}} has a duplicate ID", position)
		}
		seen[id] = struct{}{}
		name := strings.TrimSpace(rule.Name)
		if name == "" || utf8.RuneCountInString(name) > maxRevenueRuleNameLen {
			return nil, common.NewMessage("Revenue rule {{index}} needs a name of at most 100 characters", position)
		}
		if math.IsNaN(rule.PricePerTB) || math.IsInf(rule.PricePerTB, 0) || rule.PricePerTB < 0 {
			return nil, common.NewMessage("Revenue rule {{index}} needs a price that is not negative", position)
		}
		pattern := strings.TrimSpace(rule.Pattern)
		if pattern == "" || len(pattern) > maxRevenuePatternLen {
			return nil, common.NewMessage("Revenue rule {{index}} needs a pattern of at most 512 characters", position)
		}
		compiled := compiledRevenueRule{rule: rule}
		compiled.rule.ID, compiled.rule.Name, compiled.rule.Pattern = id, name, pattern
		switch rule.MatchType {
		case RevenueMatchKeyword:
			compiled.keyword = strings.ToLower(pattern)
		case RevenueMatchRegex:
			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil, common.NewMessage("Revenue rule {{index}} has an invalid regular expression", position)
			}
			compiled.regex = re
		case RevenueMatchGlob:
			glob, err := compileModel(pattern)
			if err != nil {
				return nil, common.NewMessage("Revenue rule {{index}} has an invalid glob pattern", position)
			}
			compiled.glob = glob
		default:
			return nil, common.NewMessage("Revenue rule {{index}} has an unknown match type", position)
		}
		if rule.Enabled {
			matcher.rules = append(matcher.rules, compiled)
		}
	}
	return matcher, nil
}

// Match returns the first enabled rule matching the model name.
func (m *RevenueMatcher) Match(model string) (RevenueRule, bool) {
	if m == nil {
		return RevenueRule{}, false
	}
	for _, r := range m.rules {
		if r.match(model) {
			return r.rule, true
		}
	}
	return RevenueRule{}, false
}

// Rules returns the enabled rules in priority order.
func (m *RevenueMatcher) Rules() []RevenueRule {
	if m == nil {
		return nil
	}
	out := make([]RevenueRule, 0, len(m.rules))
	for _, r := range m.rules {
		out = append(out, r.rule)
	}
	return out
}

// ParseRevenueRules decodes the stored option; an empty value is no rules.
func ParseRevenueRules(raw string) ([]RevenueRule, error) {
	rules := []RevenueRule{}
	if strings.TrimSpace(raw) == "" {
		return rules, nil
	}
	if err := common.UnmarshalJsonStr(raw, &rules); err != nil {
		return nil, common.NewMessage("Revenue rules must be a JSON array")
	}
	if rules == nil {
		rules = []RevenueRule{}
	}
	return rules, nil
}

// ValidateRevenueOption checks a revenue option before it is stored.
func ValidateRevenueOption(key, value string) error {
	switch key {
	case RevenueRulesOptionKey:
		rules, err := ParseRevenueRules(value)
		if err != nil {
			return err
		}
		_, err = CompileRevenueRules(rules)
		return err
	case RevenueCurrencyLabelOptionKey:
		if utf8.RuneCountInString(strings.TrimSpace(value)) > MaxRevenueCurrencyLabelLen {
			return common.NewMessage("The currency label can be at most 16 characters")
		}
	}
	return nil
}
