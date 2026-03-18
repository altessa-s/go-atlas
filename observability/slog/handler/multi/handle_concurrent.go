// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package multi

import (
	"context"
	"errors"
	"log/slog"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
)

// handleConcurrent dispatches the record to all enabled children concurrently
// using [concurrency.Process]. Errors are collected via [errors.Join].
func handleConcurrent(ctx context.Context, r slog.Record, handlers []slog.Handler) error {
	enabled := make([]slog.Handler, 0, len(handlers))
	for _, child := range handlers {
		if child.Enabled(ctx, r.Level) {
			enabled = append(enabled, child)
		}
	}
	if len(enabled) == 0 {
		return nil
	}

	var errs []error
	_ = concurrency.Process(ctx, enabled, func(ctx context.Context, child slog.Handler) error {
		return child.Handle(ctx, r)
	}, concurrency.BatchConfig[slog.Handler]{
		Concurrency: len(enabled),
		OnError: func(_ slog.Handler, err error) {
			errs = append(errs, err) // safe: OnError called under mutex in Process
		},
	})
	return errors.Join(errs...)
}
