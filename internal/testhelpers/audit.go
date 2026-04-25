// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import (
	"context"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/audit/storages/memory"
	"github.com/altessa-s/go-atlas/service/dispatch"
)

// ShutdownFunc stops both the auditor and the underlying dispatch engine,
// flushing any buffered events to storage. Call it before asserting on
// store contents.
type ShutdownFunc func(ctx context.Context) error

// NewTestAuditor creates an [audit.Auditor] backed by [memory.Storage] for testing.
// It applies test-friendly dispatch defaults (50ms flush interval, 1 worker, batch
// size 1) and starts both the dispatch engine and the auditor. The returned
// [ShutdownFunc] stops both components and flushes buffered events — call it
// before asserting on store contents. If not called explicitly, tb.Cleanup
// shuts everything down automatically.
func NewTestAuditor(tb testing.TB, opts ...audit.Option) (*audit.Auditor, *memory.Storage, ShutdownFunc) {
	tb.Helper()

	store := memory.New()

	eng, err := dispatch.NewEngine[*audit.Event](
		audit.StorageSink{Storage: store},
		dispatch.WithFlushInterval[*audit.Event](50*time.Millisecond), //nolint:mnd // test constant
		dispatch.WithWorkers[*audit.Event](1),
		dispatch.WithBatchSize[*audit.Event](1),
	)
	if err != nil {
		tb.Fatalf("failed to create dispatch engine: %v", err)
	}
	if err = eng.Start(); err != nil {
		tb.Fatalf("failed to start dispatch engine: %v", err)
	}

	a, err := audit.New(eng, opts...)
	if err != nil {
		tb.Fatalf("failed to create auditor: %v", err)
	}
	if err = a.Start(); err != nil {
		tb.Fatalf("failed to start auditor: %v", err)
	}

	shutdown := ShutdownFunc(func(ctx context.Context) error {
		_ = a.Shutdown(ctx) //nolint:errcheck // test helper
		return eng.Shutdown(ctx)
	})

	tb.Cleanup(func() {
		_ = shutdown(tb.Context()) //nolint:errcheck // test cleanup
	})

	return a, store, shutdown
}
