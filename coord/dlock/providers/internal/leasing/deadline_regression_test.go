// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leasing_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/altessa-s/go-atlas/coord/dlock/providers"
	"github.com/altessa-s/go-atlas/coord/dlock/providers/internal/leasing"
	"github.com/stretchr/testify/require"
)

type renewalFaultStore struct {
	*memStore
	renew func(context.Context) (bool, error)
}

func (s *renewalFaultStore) Renew(ctx context.Context, _, _ string, _ uint64) (bool, error) {
	return s.renew(ctx)
}

func TestRenewalDeadline(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"transient", "repeated", "slow", "late-success"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				attempts := 0
				s := &renewalFaultStore{memStore: newMemStore(10 * time.Second)}
				s.renew = func(ctx context.Context) (bool, error) {
					attempts++
					if mode == "slow" || mode == "late-success" {
						<-ctx.Done()
						if mode == "late-success" {
							return true, nil
						}
						return false, ctx.Err()
					}
					if mode == "transient" && attempts > 1 {
						return true, nil
					}
					return false, errors.New("unavailable")
				}
				e := leasing.New(s, leasing.Settings{Logger: slog.New(slog.DiscardHandler), TTL: 10 * time.Second, Interval: 9 * time.Second, OperationsTimeout: 5 * time.Second})
				defer func() { require.NoError(t, e.Close(t.Context())) }()
				ctx, cancel := context.WithTimeout(t.Context(), time.Hour)
				defer cancel()
				lk, err := e.Lock(ctx, "key")
				require.NoError(t, err)
				<-time.After(10*time.Second - time.Millisecond)
				synctest.Wait()
				lost := lk.(providers.LostNotifier).Lost()
				select {
				case <-lost:
					require.FailNow(t, "lease loss was signaled before the deadline")
				default:
				}
				<-time.After(time.Millisecond)
				synctest.Wait()
				if mode == "transient" {
					require.Greater(t, attempts, 1)
					select {
					case <-lost:
						require.FailNow(t, "renewed lease was lost")
					default:
					}
				} else {
					select {
					case <-lost:
					default:
						require.FailNow(t, "lease loss was not signaled at the deadline")
					}
				}
			})
		})
	}
}
