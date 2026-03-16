// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"regexp"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"
	"github.com/altessa-s/go-atlas/transport/internal/ipacl"
)

// compilePatterns compiles string patterns to regexp.
func compilePatterns(patterns []string) []*regexp.Regexp {
	result := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		if re, err := regexp.Compile(p); err == nil {
			result = append(result, re)
		}
	}
	return result
}

// parsePrefixes parses string IP/CIDR prefixes to netip.Prefix.
var parsePrefixes = clientip.ParsePrefixes

// convertFallbackBehavior converts config FallbackBehavior to internal fallback.Behavior.
func convertFallbackBehavior(fb config.FallbackBehavior) fallback.Behavior {
	return fallback.ParseBehavior(string(fb))
}

// buildIpAclRegistry builds an ipacl.Registry from configuration.
func buildIpAclRegistry(defaultPolicy string, rules []config.IpAclRuleConfig, defaultRule *config.IpAclRuleConfig) (*ipacl.Registry, error) {
	registry := ipacl.NewRegistry(ipacl.ParsePolicy(defaultPolicy))

	for _, r := range rules {
		rule, err := convertIpAclRule(r)
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
		rule, err := convertIpAclRule(*defaultRule)
		if err != nil {
			return nil, err
		}
		registry.SetDefault(rule)
	}

	return registry, nil
}

// convertIpAclRule converts a config rule to an ipacl.AccessRule.
func convertIpAclRule(r config.IpAclRuleConfig) (*ipacl.AccessRule, error) {
	rule := &ipacl.AccessRule{}

	if len(r.Allowlist) > 0 {
		prefixes, err := parsePrefixes(r.Allowlist)
		if err != nil {
			return nil, fmt.Errorf("invalid allowlist: %w", err)
		}
		rule.Allowlist = prefixes
	}

	if len(r.Denylist) > 0 {
		prefixes, err := parsePrefixes(r.Denylist)
		if err != nil {
			return nil, fmt.Errorf("invalid denylist: %w", err)
		}
		rule.Denylist = prefixes
	}

	return rule, nil
}
