// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"regexp"

	"github.com/altessa-s/go-atlas/config"
	"github.com/altessa-s/go-atlas/transport/internal/clientip"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"
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
