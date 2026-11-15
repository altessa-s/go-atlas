// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiter

//go:generate go tool optgen generate --type=options

import (
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/transport/policy/fallback"
)

// options holds configuration for the rate limiter interceptor.
type options struct {
	logger *slog.Logger
	// ignoreMethods and ignorePatterns have no defaults: rate limiting exempts
	// nothing, gRPC reflection and health included, unless the caller
	// opts in, e.g. WithIgnorePatterns(defaults.IgnorePatterns...).
	ignoreMethods    []string
	ignorePatterns   []*regexp.Regexp
	fallbackBehavior fallback.Behavior `optgen:"default=fallback.Deny"`
	exposeHeaders    bool              `optgen:"default=false"`
}
