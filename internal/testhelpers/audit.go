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
)

// NewTestAuditor creates an [audit.Auditor] backed by [memory.Storage] for testing.
// It applies test-friendly defaults (50ms flush interval, 1 worker, batch size 1)
// before appending any caller-supplied opts, so callers can override individual settings.
// The auditor is started before returning and shut down automatically via tb.Cleanup.
func NewTestAuditor(tb testing.TB, opts ...audit.Option) (*audit.Auditor, *memory.Storage) {
	tb.Helper()
	store := memory.New()
	defaults := []audit.Option{
		audit.WithFlushInterval(50 * time.Millisecond), //nolint:mnd // test constant
		audit.WithWorkers(1),
		audit.WithBatchSize(1),
	}
	defaults = append(defaults, opts...)
	a, err := audit.New(store, defaults...)
	if err != nil {
		tb.Fatalf("failed to create auditor: %v", err)
	}
	if err := a.Start(); err != nil {
		tb.Fatalf("failed to start auditor: %v", err)
	}
	tb.Cleanup(func() {
		_ = a.Shutdown(context.Background()) //nolint:errcheck // test cleanup
	})
	return a, store
}
