// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package defaults

import (
	"regexp"

	"github.com/altessa-s/go-atlas/transport/internal/endpointfilter"
)

// IgnorePatterns contains compiled regexp patterns for paths that most
// middleware should skip: health-check endpoints, metrics, and pprof.
// Middleware option structs reference this variable as their default
// ignorePatterns value via code generation tags.
var IgnorePatterns = []*regexp.Regexp{
	endpointfilter.HealthPathPattern,
	endpointfilter.MetricsPathPattern,
	endpointfilter.PprofPathPattern,
}
