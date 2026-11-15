// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

//go:generate go tool optgen generate --type=options

import (
	"context"
	"log/slog"
	"net/http"
	"regexp"
)

// PanicHandler is a function that handles panics in HTTP handlers.
// It receives the context, ResponseWriter, Request, and the recovered panic
// value. It should write an appropriate error response to the client unless
// [HeadersWritten] reports the response has already started.
// Configure with [WithHandler]; if nil, the middleware writes a bare 500
// status code.
type PanicHandler func(ctx context.Context, w http.ResponseWriter, r *http.Request, p any)

// options configures the panic recovery middleware.
type options struct {
	logger  *slog.Logger
	handler PanicHandler `optgen:"notnil"`
	// ignorePaths and ignorePatterns have no defaults: the middleware exempts
	// nothing unless the caller configures it.
	ignorePaths    []string
	ignorePatterns []*regexp.Regexp
	logStack       bool `optgen:"default=true"`
}
