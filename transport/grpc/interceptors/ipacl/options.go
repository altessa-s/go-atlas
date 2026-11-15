// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ipacl

//go:generate go tool optgen generate --type=options

import (
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/transport/policy/fallback"
)

// options holds configuration for the IP access control interceptor.
type options struct {
	logger *slog.Logger
	// ignoreMethods and ignorePatterns have no defaults: access control exempts
	// nothing, gRPC reflection and health included, unless the caller
	// opts in, e.g. WithIgnorePatterns(defaults.IgnorePatterns...).
	ignoreMethods    []string
	ignorePatterns   []*regexp.Regexp
	fallbackBehavior fallback.Behavior `optgen:"default=fallback.Deny"`
}
