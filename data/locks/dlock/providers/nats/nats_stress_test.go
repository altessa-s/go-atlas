// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build stress

package nats_test

import (
	"context"
	"errors"
	"math/rand/v2"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	locknats "github.com/altessa-s/go-atlas/data/locks/dlock/providers/nats"
)

// stressNATSURL returns the shared broker of tests/integration/docker-compose.yml,
// or NATS_URL when set.
func stressNATSURL() string {
	if v := os.Getenv("NATS_URL"); v != "" {
		return v
	}
	return "nats://127.0.0.1:14222"
}

// stressConnect dials the shared broker, skipping the test when it is
// unreachable.
func stressConnect(tb testing.TB) *nats.Conn {
	tb.Helper()
	nc, err := nats.Connect(stressNATSURL(), nats.Timeout(2*time.Second), nats.RetryOnFailedConnect(false))
	if err != nil {
		tb.Skipf("NATS unreachable at %s (%v) — start it with: docker compose -f tests/integration/docker-compose.yml up -d nats",
			stressNATSURL(), err)
	}
	tb.Cleanup(nc.Close)
	return nc
}

// TestStressLocker churns Lock, Release and context cancellation on a few
// overlapping keys through several providers sharing one bucket on a live
// broker. Some acquisitions are abandoned by canceling their context. No two
// holders may ever be inside a key's critical section together, and every
// provider must stop tracking locks once the churn ends.
func TestStressLocker(t *testing.T) {
	const (
		providers = 3
		workers   = 12
		keys      = 4
	)
	deadline := time.Now().Add(testhelpers.StressDuration(t))
	bucket := "stress-" + strconv.FormatInt(time.Now().UnixNano(), 36)

	nc := stressConnect(t)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	t.Cleanup(func() { _ = js.DeleteKeyValue(context.Background(), bucket) })

	lockers := make([]*locknats.Locker, providers)
	for i := range lockers {
		l, err := locknats.New(t.Context(), stressConnect(t), locknats.WithBucket(bucket), locknats.WithTTL(2*time.Second))
		require.NoError(t, err)
		lockers[i] = l
	}

	var (
		inside    [keys]atomic.Int32
		acquired  atomic.Int64
		contended atomic.Int64
		abandoned atomic.Int64
		wg        sync.WaitGroup
	)
	for range workers {
		wg.Go(func() {
			for time.Now().Before(deadline) && !t.Failed() {
				k := rand.IntN(keys) //nolint:gosec // test chaos
				key := "key-" + strconv.Itoa(k)
				l := lockers[rand.IntN(providers)] //nolint:gosec // test chaos
				ctx, cancel := context.WithCancel(t.Context())

				if rand.IntN(10) == 0 { //nolint:gosec // test chaos
					// Abandon the acquisition: cancel it while it may be in flight.
					time.AfterFunc(time.Duration(rand.IntN(2000))*time.Microsecond, cancel) //nolint:gosec // test chaos
					if lk, err := l.Lock(ctx, key); err == nil {
						_ = lk.Release(context.Background()) //nolint:errcheck // the lease may already be gone
					}
					cancel()
					abandoned.Add(1)
					continue
				}

				lk, err := l.Lock(ctx, key)
				if errors.Is(err, errs.ErrLockNotHeld) {
					cancel()
					contended.Add(1)
					time.Sleep(time.Millisecond)
					continue
				}
				if err != nil {
					cancel()
					t.Errorf("Lock(%s): %v", key, err)
					return
				}
				acquired.Add(1)
				if n := inside[k].Add(1); n != 1 {
					t.Errorf("%s: %d holders inside the critical section", key, n)
				}
				time.Sleep(time.Duration(rand.IntN(3000)) * time.Microsecond) //nolint:gosec // test chaos
				inside[k].Add(-1)

				switch rand.IntN(10) { //nolint:gosec // test chaos
				case 0, 1:
					cancel() // release through the context only
				case 2:
					cancel()
					_ = lk.Release(context.Background()) //nolint:errcheck // may race the context release
				default:
					if err := lk.Release(ctx); err != nil {
						t.Errorf("Release(%s): %v", key, err)
					}
					cancel()
				}
			}
		})
	}
	wg.Wait()
	require.False(t, t.Failed(), "invariant violated during the run")
	require.Positive(t, acquired.Load(), "no lock was ever acquired")

	for i, l := range lockers {
		require.Eventually(t, func() bool { return l.TrackedLocks() == 0 }, 10*time.Second, 10*time.Millisecond,
			"provider %d still tracks %d locks", i, l.TrackedLocks())
		require.NoError(t, l.Close(t.Context()))
	}
	t.Logf("acquired=%d contended=%d abandoned=%d", acquired.Load(), contended.Load(), abandoned.Load())
}
