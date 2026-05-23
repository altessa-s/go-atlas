// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package geoacl

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/defaults"
	"github.com/altessa-s/go-atlas/transport/internal/fallback"
)

// Use defaults package for optgen code generation
var _ = defaults.IgnorePatterns

// options holds configuration for the geographic access control middleware.
type options struct {
	logger           *slog.Logger
	ignorePaths      []string
	ignorePatterns   []*regexp.Regexp  `optgen:"default=defaults.IgnorePatterns"`
	fallbackBehavior fallback.Behavior `optgen:"default=fallback.Deny"`
}
