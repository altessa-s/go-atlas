// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package endpointrule

import "regexp"

type patternRule[R any] struct {
	pattern *regexp.Regexp
	rule    *R
}

// Registry holds rules keyed by exact endpoint name or regex pattern, plus an
// optional default rule. The zero value is ready to use.
type Registry[R any] struct {
	rules        map[string]*R
	patternRules []patternRule[R]
	defaultRule  *R
}

// Register adds an exact-match rule for the given endpoint.
func (r *Registry[R]) Register(endpoint string, rule *R) {
	if r.rules == nil {
		r.rules = make(map[string]*R)
	}
	r.rules[endpoint] = rule
}

// RegisterPattern adds a regex-based rule. Patterns are tried in registration
// order.
func (r *Registry[R]) RegisterPattern(pattern *regexp.Regexp, rule *R) {
	r.patternRules = append(r.patternRules, patternRule[R]{
		pattern: pattern,
		rule:    rule,
	})
}

// RegisterEndpoints adds the same rule for multiple endpoints.
func (r *Registry[R]) RegisterEndpoints(rule *R, endpoints ...string) {
	for _, ep := range endpoints {
		r.Register(ep, rule)
	}
}

// SetDefault sets the fallback rule applied when no exact or pattern match is found.
func (r *Registry[R]) SetDefault(rule *R) {
	r.defaultRule = rule
}

// Lookup returns the rule for the given endpoint.
// It checks exact matches first, then patterns, then the default rule.
// The boolean indicates whether any rule was found.
func (r *Registry[R]) Lookup(endpoint string) (*R, bool) {
	if rule, ok := r.rules[endpoint]; ok {
		return rule, true
	}
	for _, pr := range r.patternRules {
		if pr.pattern.MatchString(endpoint) {
			return pr.rule, true
		}
	}
	if r.defaultRule != nil {
		return r.defaultRule, true
	}
	return nil, false
}
