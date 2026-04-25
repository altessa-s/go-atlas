// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package logger

import (
	"log/slog"
	"regexp"

	"github.com/altessa-s/go-atlas/core/time/timeformat"
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/defaults"
)

// Use defaults package for optgen code generation
var _ = defaults.IgnorePatterns

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

// BodyRedactFunc is called to redact sensitive data from request/response body
// content before it is written to logs. It receives the raw body string and
// must return the redacted version. Applied only when [WithLogRequest] or
// [WithLogResponse] is enabled. Configure with [WithBodyRedactor].
type BodyRedactFunc func(body string) string

// options holds configuration for the logger middleware.
type options struct {
	timeFormat          timeformat.Format `optgen:"default=timeformat.RFC3339"`
	logResponse         bool
	logRequest          bool
	ignorePaths         []string
	ignorePatterns      []*regexp.Regexp `optgen:"default=defaults.IgnorePatterns"`
	ignoreMethods       []string         `optgen:"mod=upper"`
	logResponseCodes    []int
	ignoreResponseCodes []int
	// contextLogger enables storing enriched logger in context.
	// Handlers can retrieve it via slogx.FromContextOrDefault(ctx).
	// If nil, context injection is disabled (default behavior).
	contextLogger *slog.Logger   `optgen:"default=nil"`
	bodyRedactor  BodyRedactFunc `optgen:"default=nil"`
}
