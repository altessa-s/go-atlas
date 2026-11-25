// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"log/slog"
)

// SlogAdapter implements the logger interface accepted by go-redis.
// Its zero value discards messages.
type SlogAdapter struct {
	logger *slog.Logger
}

// NewSlogAdapter adapts logger for use with redis.SetLogger. Install it once
// during service startup. Use a nonblocking handler to avoid delaying Redis
// operations that log while holding locks. A nil logger discards messages.
func NewSlogAdapter(logger *slog.Logger) SlogAdapter {
	return SlogAdapter{logger: logger}
}

// Printf reports a go-redis diagnostic at WARN level. The raw diagnostic may
// contain server addresses or error text, so it is omitted.
func (a SlogAdapter) Printf(ctx context.Context, _ string, _ ...any) {
	if a.logger != nil {
		a.logger.WarnContext(ctx, "go-redis emitted a diagnostic")
	}
}
