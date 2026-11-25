// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package realip

//go:generate go tool optgen generate --type=options

import (
	"log/slog"
)

// options holds configuration for the realip interceptor.
type options struct {
	// logClientDetails enables logging client details that may identify callers.
	logClientDetails bool `optval:"param"`
	// logRawErrors enables logging error text, panic values, or stacks that may contain secrets.
	logRawErrors bool `optval:"param"`

	logger *slog.Logger
}
