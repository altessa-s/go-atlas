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

// options holds configuration for the rate limiter middleware.
type options struct {
	logger *slog.Logger
	// ignorePaths and ignorePatterns have no defaults: the middleware exempts
	// nothing unless the caller configures it.
	ignorePaths      []string
	ignorePatterns   []*regexp.Regexp
	fallbackBehavior fallback.Behavior
}
