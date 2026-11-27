// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgresit_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/altessa-s/go-atlas/store/postgres"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"
)

func TestAdvisoryLock(t *testing.T) {
	target := livebackend.SQLTargets()[0]
	holder := target.Open(t)
	contender := target.Open(t)

	for _, ending := range []struct {
		name string
		err  error
	}{
		{name: "commit"},
		{name: "rollback", err: errors.New("roll back lock holder")},
	} {
		t.Run(ending.name, func(t *testing.T) {
			testCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			key := livebackend.UniqueName(t, "advisory_lock")
			var contenderDone chan error

			holderErr := postgres.WithTransaction(testCtx, holder, func(ctx context.Context) error {
				if err := postgres.AdvisoryLock(ctx, key); err != nil {
					return err
				}
				pid := make(chan int, 1)
				contenderDone = make(chan error, 1)
				go func() {
					// Use the test context as the parent so this transaction owns a separate connection.
					contenderDone <- postgres.WithTransaction(testCtx, contender, func(other context.Context) error {
						tx, _, _ := postgres.Tx(other)
						var backendPID int
						if err := tx.QueryRowContext(other, "SELECT pg_backend_pid()").Scan(&backendPID); err != nil {
							return err
						}
						if err := postgres.AdvisoryLock(other, key+"/other"); err != nil {
							return fmt.Errorf("lock a different key while holder is active: %w", err)
						}
						pid <- backendPID
						return postgres.AdvisoryLock(other, key)
					})
				}()

				select {
				case backendPID := <-pid:
					tx, _, _ := postgres.Tx(ctx)
					if err := waitForAdvisoryLockWaiter(ctx, tx, backendPID); err != nil {
						return err
					}
					return ending.err
				case <-ctx.Done():
					return fmt.Errorf("contender did not acquire a different key before holder ended: %w", ctx.Err())
				}
			})
			if contenderDone != nil {
				select {
				case err := <-contenderDone:
					if err != nil {
						t.Errorf("contender failed to acquire locks: %v", err)
					}
				case <-testCtx.Done():
					t.Errorf("contender did not finish after holder ended: %v", testCtx.Err())
				}
			}
			if !errors.Is(holderErr, ending.err) {
				t.Errorf("lock holder ended with %v, want %v", holderErr, ending.err)
			}
		})
	}
}

func waitForAdvisoryLockWaiter(ctx context.Context, holder *sql.Tx, pid int) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		// PostgreSQL identifies the active blocker, so scheduling delays cannot look like lock contention.
		var blocked bool
		err := holder.QueryRowContext(ctx, "SELECT pg_backend_pid() = ANY(pg_blocking_pids($1))", pid).Scan(&blocked)
		if err != nil || blocked {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("contender never waited on the holder's key: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}
