// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

//go:generate go tool optgen generate --type=options

import (
	"context"
	"log/slog"
	"regexp"
)

// PanicHandler is a function that handles panics.
type PanicHandler func(ctx context.Context, p any) error

// options holds configuration for the recovery interceptor.
type options struct {
	panicHandler PanicHandler
	logger       *slog.Logger
	// ignoreMethods and ignorePatterns have no defaults: the middleware exempts
	// nothing unless the caller configures it.
	ignoreMethods  []string
	ignorePatterns []*regexp.Regexp
}
