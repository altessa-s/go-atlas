// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factoryconv

import (
	"fmt"
	"regexp"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"
	"github.com/altessa-s/go-atlas/transport/internal/geoacl"
	"github.com/altessa-s/go-atlas/transport/internal/ipacl"
)

// CompilePatterns compiles string patterns to regexp.
// Invalid patterns are silently skipped.
func CompilePatterns(patterns []string) []*regexp.Regexp {
	result := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			result = append(result, re)
		}
	}
	return result
}

// ParsePrefixes parses string IP/CIDR prefixes to netip.Prefix.
var ParsePrefixes = clientip.ParsePrefixes

// ConvertFallbackBehavior converts config FallbackBehavior to internal fallback.Behavior.
func ConvertFallbackBehavior(fb config.FallbackBehavior) fallback.Behavior {
	return fallback.ParseBehavior(string(fb))
}

// BuildIpAclRegistry builds an ipacl.Registry from configuration.
func BuildIpAclRegistry(defaultPolicy string, rules []config.IpAclRuleConfig, defaultRule *config.IpAclRuleConfig) (*ipacl.Registry, error) {
	registry := ipacl.NewRegistry(ipacl.ParsePolicy(defaultPolicy))

	for _, r := range rules {
		rule, err := ConvertIpAclRule(r)
		if err != nil {
			return nil, err
		}

		for _, ep := range r.Endpoints {
			registry.Register(ep, rule)
		}

		for _, p := range r.Patterns {
			re, err := regexp.Compile(p)
			if err != nil {
				return nil, fmt.Errorf("invalid pattern %q: %w", p, err)
			}
			registry.RegisterPattern(re, rule)
		}
	}

	if defaultRule != nil {
		rule, err := ConvertIpAclRule(*defaultRule)
		if err != nil {
			return nil, err
		}
		registry.SetDefault(rule)
	}

	return registry, nil
}

// ConvertIpAclRule converts a config rule to an ipacl.AccessRule.
func ConvertIpAclRule(r config.IpAclRuleConfig) (*ipacl.AccessRule, error) {
	rule := &ipacl.AccessRule{}

	if len(r.Allowlist) > 0 {
		prefixes, err := ParsePrefixes(r.Allowlist)
		if err != nil {
			return nil, fmt.Errorf("invalid allowlist: %w", err)
		}
		rule.Allowlist = prefixes
	}

	if len(r.Denylist) > 0 {
		prefixes, err := ParsePrefixes(r.Denylist)
		if err != nil {
			return nil, fmt.Errorf("invalid denylist: %w", err)
		}
		rule.Denylist = prefixes
	}

	return rule, nil
}

// BuildGeoAclRegistry builds a geoacl.Registry from configuration.
func BuildGeoAclRegistry(defaultPolicy string, rules []config.GeoAclRuleConfig, defaultRule *config.GeoAclRuleConfig) (*geoacl.Registry, error) {
	registry := geoacl.NewRegistry(geoacl.ParsePolicy(defaultPolicy))

	for _, r := range rules {
		rule := ConvertGeoAclRule(r)

		for _, ep := range r.Endpoints {
			registry.Register(ep, rule)
		}

		for _, p := range r.Patterns {
			re, err := regexp.Compile(p)
			if err != nil {
				return nil, fmt.Errorf("invalid pattern %q: %w", p, err)
			}
			registry.RegisterPattern(re, rule)
		}
	}

	if defaultRule != nil {
		rule := ConvertGeoAclRule(*defaultRule)
		registry.SetDefault(rule)
	}

	return registry, nil
}

// ConvertGeoAclRule converts a config rule to a geoacl.AccessRule.
func ConvertGeoAclRule(r config.GeoAclRuleConfig) *geoacl.AccessRule {
	return &geoacl.AccessRule{
		AllowContinents: r.AllowContinents,
		DenyContinents:  r.DenyContinents,
		AllowCountries:  r.AllowCountries,
		DenyCountries:   r.DenyCountries,
		AllowRegions:    r.AllowRegions,
		DenyRegions:     r.DenyRegions,
	}
}
