// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package defaults

import (
	"regexp"

	"github.com/altessa-s/go-atlas/internal/transport/endpointfilter"
)

// IgnorePatterns contains compiled regexp patterns for the health-check,
// metrics, and pprof endpoints that observability middleware (logger,
// tracing, metrics, ...) skips by default. The patterns are anchored: they
// match the endpoints at the root or directly under the server factory's
// default "/internal" prefix, never an application route that merely ends in
// the same segment. Middleware option structs reference this variable as
// their default ignorePatterns value via code generation tags.
//
// Access-control and safety middleware (auth, ipacl, geoacl, limiter,
// recovery) does not use it by default; opt in explicitly.
var IgnorePatterns = []*regexp.Regexp{
	endpointfilter.HealthPathPattern,
	endpointfilter.MetricsPathPattern,
	endpointfilter.PprofPathPattern,
}

// IgnorePatternsWithPrefix returns the patterns of [IgnorePatterns] for
// built-in endpoints mounted under prefix instead of "/internal". An empty
// prefix matches root-level endpoints only.
func IgnorePatternsWithPrefix(prefix string) []*regexp.Regexp {
	return endpointfilter.InfrastructurePathPatterns(prefix)
}
