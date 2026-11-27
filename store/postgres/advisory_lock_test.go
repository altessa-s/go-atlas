// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

func TestAdvisoryLockKey(t *testing.T) {
	t.Parallel()

	const want int64 = -91948667103783825
	if got := AdvisoryLockKey("integration-kind/SMTP"); got != want {
		t.Errorf("AdvisoryLockKey(integration-kind/SMTP) = %d, want %d", got, want)
	}
}

func TestAdvisoryLockRequiresWriteTransaction(t *testing.T) {
	t.Parallel()

	db, fake := testhelpers.NewFakeSQL(t, nil)
	lock := func(ctx context.Context) error { return AdvisoryLock(ctx, "test") }

	if err := lock(t.Context()); !errors.Is(err, ErrWriteTransactionRequired) {
		t.Errorf("AdvisoryLock outside a transaction = %v, want ErrWriteTransactionRequired", err)
	}
	if err := WithSnapshot(t.Context(), db, lock); !errors.Is(err, ErrWriteTransactionRequired) {
		t.Errorf("AdvisoryLock in a snapshot = %v, want ErrWriteTransactionRequired", err)
	}

	var finished context.Context
	if err := WithTransaction(t.Context(), db, func(ctx context.Context) error {
		finished = ctx
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := lock(finished); !errors.Is(err, ErrWriteTransactionRequired) {
		t.Errorf("AdvisoryLock after commit = %v, want ErrWriteTransactionRequired", err)
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Errorf("AdvisoryLock ran SQL without an open write transaction: %v", calls)
	}
}
