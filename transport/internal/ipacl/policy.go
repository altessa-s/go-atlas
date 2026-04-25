// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ipacl

import (
	"net/netip"
	"regexp"

	"github.com/altessa-s/go-atlas/transport/internal/clientip"
)

// Policy determines the default action when an IP matches neither the
// allowlist nor the denylist of a rule.
type Policy int

const (
	// PolicyDeny denies access by default (allowlist mode).
	// Only IPs explicitly listed in allowlists are permitted.
	PolicyDeny Policy = iota

	// PolicyAllow allows access by default (denylist mode).
	// Only IPs explicitly listed in denylists are blocked.
	PolicyAllow
)

// AccessRule defines allowlist and denylist CIDR prefixes for an endpoint or
// group of endpoints.
type AccessRule struct {
	Allowlist []netip.Prefix
	Denylist  []netip.Prefix
}

type patternRule struct {
	pattern *regexp.Regexp
	rule    *AccessRule
}

// Registry holds a collection of access rules keyed by endpoint name or
// regex pattern, plus an optional default rule and a fallback policy.
type Registry struct {
	rules        map[string]*AccessRule
	patternRules []patternRule
	defaultRule  *AccessRule
	policy       Policy
}

// NewRegistry creates a new Registry with the given default policy.
func NewRegistry(policy Policy) *Registry {
	return &Registry{
		rules:  make(map[string]*AccessRule),
		policy: policy,
	}
}

// Register adds an exact-match access rule for the given endpoint.
func (r *Registry) Register(endpoint string, rule *AccessRule) {
	r.rules[endpoint] = rule
}

// RegisterPattern adds a regex-based access rule.
func (r *Registry) RegisterPattern(pattern *regexp.Regexp, rule *AccessRule) {
	r.patternRules = append(r.patternRules, patternRule{
		pattern: pattern,
		rule:    rule,
	})
}

// RegisterEndpoints adds the same access rule for multiple endpoints.
func (r *Registry) RegisterEndpoints(rule *AccessRule, endpoints ...string) {
	for _, ep := range endpoints {
		r.rules[ep] = rule
	}
}

// SetDefault sets the fallback rule applied when no exact or pattern match is found.
func (r *Registry) SetDefault(rule *AccessRule) {
	r.defaultRule = rule
}

// Lookup returns the access rule for the given endpoint.
// It checks exact matches first, then patterns, then the default rule.
// The boolean indicates whether any rule was found.
func (r *Registry) Lookup(endpoint string) (*AccessRule, bool) {
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

// Evaluate checks whether the given IP address is allowed to access
// the given endpoint.
//
// IPv4-mapped IPv6 addresses (e.g. ::ffff:10.0.0.1) are automatically
// unmapped to their IPv4 form before matching, so IPv4 CIDR prefixes
// work correctly in dual-stack environments.
//
// Decision logic within a matched rule:
//  1. IP in Denylist  -> deny  (deny wins)
//  2. IP in Allowlist -> allow
//  3. Neither         -> fall back to Registry.policy
//
// If no rule matches the endpoint, the registry-level policy applies directly.
func (r *Registry) Evaluate(ip netip.Addr, endpoint string) bool {
	// Normalize IPv4-mapped IPv6 addresses so that ::ffff:10.0.0.1
	// matches the 10.0.0.0/8 prefix.
	ip = ip.Unmap()

	rule, found := r.Lookup(endpoint)
	if !found {
		return r.policy == PolicyAllow
	}

	if clientip.InList(ip, rule.Denylist) {
		return false
	}
	if clientip.InList(ip, rule.Allowlist) {
		return true
	}

	return r.policy == PolicyAllow
}
