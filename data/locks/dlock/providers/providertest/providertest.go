// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package providertest

import (
	"context"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/locks/dlock/errs"
	"github.com/altessa-s/go-atlas/data/locks/dlock/providers"
)

// Backend is one isolated lock namespace — a table, collection or bucket no
// other test uses — and the means to reach it.
type Backend struct {
	// NewProvider returns a provider over the namespace; providers from one
	// Backend contend for the same keys, like replicas of a service. The
	// providers' TTL should be short (about a second): the renewal contract
	// holds a lock for several TTLs.
	NewProvider func(tb testing.TB) providers.Provider
	// Expire ends the stored lease of key at once, as if its holder had
	// stopped renewing long ago, without telling the holder. Nil skips the
	// contracts that need it.
	Expire func(tb testing.TB, key string)
}

// contracts lists the provider contracts; [Run] executes each.
var contracts = []struct {
	name   string
	expiry bool // needs Backend.Expire
	check  func(*testing.T, Backend)
}{
	{"Exclusive", false, Exclusive},
	{"ConcurrentLock", false, ConcurrentLock},
	{"ReleaseHandsOver", false, ReleaseHandsOver},
	{"FencingMonotonic", false, FencingMonotonic},
	{"LockInfo", false, LockInfo},
	{"Renewal", false, Renewal},
	{"ContextEndReleases", false, ContextEndReleases},
	{"FailedReleaseIsRetryable", false, FailedReleaseIsRetryable},
	{"CloseReleasesAndRejects", false, CloseReleasesAndRejects},
	{"CloseRacingLock", false, CloseRacingLock},
	{"ExpiredLeaseIsTakenOver", true, ExpiredLeaseIsTakenOver},
	{"ExpiredLeaseStaysLost", true, ExpiredLeaseStaysLost},
}

// Run executes every provider contract as a parallel subtest named after it,
// each on a fresh backend from newBackend, which must return a namespace
// isolated from every other one it returns. The contracts named in skip, and
// those needing [Backend.Expire] when it is nil, are reported as skipped.
func Run(t *testing.T, newBackend func(tb testing.TB) Backend, skip ...string) {
	t.Helper()
	for _, c := range contracts {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if slices.Contains(skip, c.name) {
				t.Skip("not supported by this backend")
			}
			b := newBackend(t)
			if c.expiry && b.Expire == nil {
				t.Skip("the backend cannot expire a lease")
			}
			c.check(t, b)
		})
	}
}

// lockOK takes key and registers its release on cleanup.
func lockOK(t *testing.T, p providers.Provider, key string) providers.Lock {
	t.Helper()
	lk, err := p.Lock(t.Context(), key)
	require.NoError(t, err, key)
	t.Cleanup(func() { _ = lk.Release(context.Background()) })
	return lk
}

// info returns the lock's own lease.
func info(t *testing.T, lk providers.Lock) *providers.LockInfo {
	t.Helper()
	inf, err := lk.GetLockInfo(t.Context())
	require.NoError(t, err)
	return inf
}

// Exclusive verifies that a held key cannot be taken again — through another
// provider or the same one — while other keys stay free.
func Exclusive(t *testing.T, b Backend) {
	t.Helper()
	p, q := b.NewProvider(t), b.NewProvider(t)
	lockOK(t, p, "k")

	_, err := q.Lock(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld)
	_, err = p.Lock(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld, "a held key is exclusive within one provider too")

	lockOK(t, q, "other")
}

// ConcurrentLock verifies that exactly one of many concurrent acquisitions of
// one key, spread over several providers, wins and the others get
// [errs.ErrLockNotHeld].
func ConcurrentLock(t *testing.T, b Backend) {
	t.Helper()
	ps := []providers.Provider{b.NewProvider(t), b.NewProvider(t), b.NewProvider(t)}

	var (
		wins atomic.Int32
		mu   sync.Mutex
		lost []error
		wg   sync.WaitGroup
	)
	for i := range 16 {
		wg.Go(func() {
			lk, err := ps[i%len(ps)].Lock(t.Context(), "race")
			if err == nil {
				wins.Add(1)
				t.Cleanup(func() { _ = lk.Release(context.Background()) })
				return
			}
			mu.Lock()
			lost = append(lost, err)
			mu.Unlock()
		})
	}
	wg.Wait()
	require.Equal(t, int32(1), wins.Load(), "exactly one concurrent Lock must win")
	for _, err := range lost {
		require.ErrorIs(t, err, errs.ErrLockNotHeld)
	}
}

// ReleaseHandsOver verifies that a release frees the key at once — for
// another provider, with a higher fencing token — and that releasing again is
// a no-op.
func ReleaseHandsOver(t *testing.T, b Backend) {
	t.Helper()
	p, q := b.NewProvider(t), b.NewProvider(t)

	lkA := lockOK(t, p, "k")
	first := info(t, lkA).FencingToken
	require.NoError(t, lkA.Release(t.Context()))
	require.NoError(t, lkA.Release(t.Context()), "releasing a released lock is a no-op")

	_, err := p.GetLockInfo(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld)
	_, err = lkA.GetLockInfo(t.Context())
	require.ErrorIs(t, err, errs.ErrLockNotHeld)

	lkB := lockOK(t, q, "k")
	require.Greater(t, info(t, lkB).FencingToken, first)
}

// FencingMonotonic verifies that the fencing token of a key strictly grows
// across successive acquisitions.
func FencingMonotonic(t *testing.T, b Backend) {
	t.Helper()
	ps := []providers.Provider{b.NewProvider(t), b.NewProvider(t)}
	var last uint64
	for i := range 6 {
		lk := lockOK(t, ps[i%len(ps)], "k")
		token := info(t, lk).FencingToken
		require.Greater(t, token, last, "acquisition %d", i)
		last = token
		require.NoError(t, lk.Release(t.Context()))
	}
}

// LockInfo verifies that the provider and the lock report the same lease —
// key, owner, fencing token, TTL and timestamps — and that a free key has
// none.
func LockInfo(t *testing.T, b Backend) {
	t.Helper()
	p, q := b.NewProvider(t), b.NewProvider(t)

	_, err := p.GetLockInfo(t.Context(), "free")
	require.ErrorIs(t, err, errs.ErrLockNotHeld)

	lk := lockOK(t, p, "k")
	own := info(t, lk)
	require.Equal(t, "k", own.Key)
	require.NotEmpty(t, own.Owner)
	require.NotZero(t, own.FencingToken)
	require.Positive(t, own.TTL)
	require.False(t, own.AcquiredAt.IsZero())
	require.False(t, own.LastRenewed.Before(own.AcquiredAt))

	seen, err := q.GetLockInfo(t.Context(), "k")
	require.NoError(t, err)
	require.Equal(t, own.Owner, seen.Owner)
	require.Equal(t, own.FencingToken, seen.FencingToken)
	require.Equal(t, own.TTL, seen.TTL)
}

// Renewal verifies that a held lock outlives its TTL: it is renewed in the
// background and stays exclusive.
func Renewal(t *testing.T, b Backend) {
	t.Helper()
	p, q := b.NewProvider(t), b.NewProvider(t)
	lk := lockOK(t, p, "k")
	first := info(t, lk)

	time.Sleep(first.TTL*2 + first.TTL/2)

	_, err := q.Lock(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld, "a renewed lock stays held past its TTL")
	now := info(t, lk)
	require.GreaterOrEqual(t, now.FencingToken, first.FencingToken, "renewal never lowers the fencing token")
	require.True(t, now.LastRenewed.After(first.LastRenewed), "the lease must have been renewed")
}

// ContextEndReleases verifies that ending the context a lock was taken with
// releases it.
//
//nolint:mnd // Fixed durations describe the provider contract.
func ContextEndReleases(t *testing.T, b Backend) {
	t.Helper()
	p, q := b.NewProvider(t), b.NewProvider(t)

	ctx, cancel := context.WithCancel(t.Context())
	_, err := p.Lock(ctx, "k")
	require.NoError(t, err)
	cancel()
	require.Eventually(t, func() bool {
		_, err := q.GetLockInfo(t.Context(), "k")
		return err != nil
	}, 5*time.Second, 20*time.Millisecond)
	lockOK(t, q, "k")
}

// FailedReleaseIsRetryable verifies that a release that failed (its context
// already ended) keeps the lease, and a later release succeeds.
func FailedReleaseIsRetryable(t *testing.T, b Backend) {
	t.Helper()
	p := b.NewProvider(t)
	lk := lockOK(t, p, "k")

	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	require.Error(t, lk.Release(canceled))
	_, err := p.GetLockInfo(t.Context(), "k")
	require.NoError(t, err, "a failed release keeps the lease")

	require.NoError(t, lk.Release(t.Context()))
	_, err = p.GetLockInfo(t.Context(), "k")
	require.ErrorIs(t, err, errs.ErrLockNotHeld)
}

// CloseReleasesAndRejects verifies that Close releases every lock held through
// the provider and rejects further acquisitions.
func CloseReleasesAndRejects(t *testing.T, b Backend) {
	t.Helper()
	p, q := b.NewProvider(t), b.NewProvider(t)

	_, err := p.Lock(t.Context(), "a")
	require.NoError(t, err)
	_, err = p.Lock(t.Context(), "b")
	require.NoError(t, err)
	require.NoError(t, p.Close(t.Context()))

	for _, key := range []string{"a", "b"} {
		_, err = q.GetLockInfo(t.Context(), key)
		require.ErrorIs(t, err, errs.ErrLockNotHeld, key)
	}
	_, err = p.Lock(t.Context(), "c")
	require.Error(t, err, "a closed provider must reject Lock")
}

// CloseRacingLock verifies that no lock acquired while Close runs is left
// held.
func CloseRacingLock(t *testing.T, b Backend) {
	t.Helper()
	p, q := b.NewProvider(t), b.NewProvider(t)

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			_, _ = p.Lock(context.Background(), "k-"+strconv.Itoa(i)) // released by Close or by Lock itself
		})
	}
	require.NoError(t, p.Close(t.Context()))
	wg.Wait()

	for i := range 20 {
		_, err := q.GetLockInfo(t.Context(), "k-"+strconv.Itoa(i))
		require.ErrorIs(t, err, errs.ErrLockNotHeld, "k-%d", i)
	}
}

// ExpiredLeaseIsTakenOver verifies that a lease which expired (a stalled
// holder) can be taken by another provider with a higher fencing token, and
// that the stale holder can neither see nor release the new lease.
func ExpiredLeaseIsTakenOver(t *testing.T, b Backend) {
	t.Helper()
	p, q := b.NewProvider(t), b.NewProvider(t)

	lkA := lockOK(t, p, "k")
	first := info(t, lkA).FencingToken
	b.Expire(t, "k")

	lkB := lockOK(t, q, "k")
	second := info(t, lkB).FencingToken
	require.Greater(t, second, first)

	_, err := lkA.GetLockInfo(t.Context())
	require.ErrorIs(t, err, errs.ErrLockNotHeld, "the stale holder must not see the new holder's lease")
	require.NoError(t, lkA.Release(t.Context()))
	seen, err := q.GetLockInfo(t.Context(), "k")
	require.NoError(t, err, "the stale holder must not release the new lease")
	require.Equal(t, second, seen.FencingToken)
}

// ExpiredLeaseStaysLost verifies that a lease which expired without being
// taken over stays lost: renewal does not revive it, and the next acquisition
// gets a higher fencing token.
//
//nolint:mnd // Fixed durations describe the provider contract.
func ExpiredLeaseStaysLost(t *testing.T, b Backend) {
	t.Helper()
	p := b.NewProvider(t)

	lk := lockOK(t, p, "k")
	inf := info(t, lk)
	b.Expire(t, "k")

	time.Sleep(inf.TTL / 2) // past the next renewal
	_, err := lk.GetLockInfo(t.Context())
	require.ErrorIs(t, err, errs.ErrLockNotHeld, "renewal must not revive an expired lease")

	next := lockOK(t, p, "k")
	require.Greater(t, info(t, next).FencingToken, inf.FencingToken)
}
