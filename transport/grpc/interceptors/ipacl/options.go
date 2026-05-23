// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ipacl

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import (
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/transport/internal/fallback"

	_ "github.com/altessa-s/go-atlas/transport/grpc/interceptors/defaults"
)

// options holds configuration for the IP access control interceptor.
type options struct {
	logger           *slog.Logger
	ignoreMethods    []string
	ignorePatterns   []*regexp.Regexp  `optgen:"default=defaults.IgnorePatterns"`
	fallbackBehavior fallback.Behavior `optgen:"default=fallback.Deny"`
}
