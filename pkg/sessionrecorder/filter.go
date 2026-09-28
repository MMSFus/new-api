package sessionrecorder

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// FilterRule selects traffic. Empty fields match anything; ChannelID 0
// means any channel. Model accepts an exact name, a "prefix*" pattern or a
// glob (path.Match syntax: *, ?, [...]).
type FilterRule struct {
	ChannelID int    `json:"channel_id,omitempty"`
	Group     string `json:"group,omitempty"`
	Model     string `json:"model,omitempty"`
}

// FilterConfig: a request is recorded when (include is empty or any include
// rule matches) and no exclude rule matches.
type FilterConfig struct {
	Include []FilterRule `json:"include"`
	Exclude []FilterRule `json:"exclude"`
}

// RequestInfo is what the filter sees about a finished relay request.
type RequestInfo struct {
	ChannelID int
	Group     string
	Model     string
}

type modelMatcher struct {
	kind    uint8 // 0 any, 1 exact, 2 prefix, 3 glob
	pattern string
}

func (m modelMatcher) match(model string) bool {
	switch m.kind {
	case 0:
		return true
	case 1:
		return model == m.pattern
	case 2:
		return strings.HasPrefix(model, m.pattern)
	default:
		ok, _ := path.Match(m.pattern, model)
		return ok
	}
}

type compiledRule struct {
	channelID int
	group     string
	model     modelMatcher
}

func (r compiledRule) match(info RequestInfo) bool {
	if r.channelID != 0 && r.channelID != info.ChannelID {
		return false
	}
	if r.group != "" && r.group != info.Group {
		return false
	}
	return r.model.match(info.Model)
}

// Filter is an immutable compiled FilterConfig; matching allocates nothing.
type Filter struct {
	include []compiledRule
	exclude []compiledRule
}

func compileModel(pattern string) (modelMatcher, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "*" {
		return modelMatcher{}, nil
	}
	star := strings.IndexAny(pattern, "*?[")
	if star < 0 {
		return modelMatcher{kind: 1, pattern: pattern}, nil
	}
	if star == len(pattern)-1 && pattern[star] == '*' {
		return modelMatcher{kind: 2, pattern: pattern[:star]}, nil
	}
	if _, err := path.Match(pattern, ""); err != nil {
		return modelMatcher{}, err
	}
	return modelMatcher{kind: 3, pattern: pattern}, nil
}

func compileRules(kind string, rules []FilterRule) ([]compiledRule, error) {
	out := make([]compiledRule, 0, len(rules))
	for i, rule := range rules {
		if rule.ChannelID < 0 {
			return nil, fmt.Errorf("filter.%s[%d].channel_id: must not be negative", kind, i)
		}
		m, err := compileModel(rule.Model)
		if err != nil {
			return nil, fmt.Errorf("filter.%s[%d].model: invalid pattern", kind, i)
		}
		out = append(out, compiledRule{channelID: rule.ChannelID, group: strings.TrimSpace(rule.Group), model: m})
	}
	return out, nil
}

// CompileFilter validates and compiles a filter configuration.
func CompileFilter(cfg FilterConfig) (*Filter, error) {
	if len(cfg.Include)+len(cfg.Exclude) > 1000 {
		return nil, errors.New("filter: too many rules")
	}
	include, err := compileRules("include", cfg.Include)
	if err != nil {
		return nil, err
	}
	exclude, err := compileRules("exclude", cfg.Exclude)
	if err != nil {
		return nil, err
	}
	return &Filter{include: include, exclude: exclude}, nil
}

// Match reports whether the request should be recorded.
func (f *Filter) Match(info RequestInfo) bool {
	if f == nil {
		return false
	}
	for _, r := range f.exclude {
		if r.match(info) {
			return false
		}
	}
	if len(f.include) == 0 {
		return true
	}
	for _, r := range f.include {
		if r.match(info) {
			return true
		}
	}
	return false
}
