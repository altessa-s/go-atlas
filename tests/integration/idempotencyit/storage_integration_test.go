// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotencyit_test

import (
	"crypto/rand"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency/storages"
)

// TestStorage_AttemptLock pins the lock itself: the first caller acquires
// and gets a token, the next one is refused and sees the holder's bytes.
func TestStorage_AttemptLock(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		s := b.open(t, longTTL)
		ctx := t.Context()

		acquired, existing, token, err := s.AttemptLock(ctx, "order-1", payload("first"))
		require.NoError(t, err)
		require.True(t, acquired)
		require.Nil(t, existing)
		require.NotEmpty(t, token)

		acquired, existing, token, err = s.AttemptLock(ctx, "order-1", payload("second"))
		require.NoError(t, err)
		require.False(t, acquired)
		require.Equal(t, payload("first"), existing, "the loser must see the holder's exact bytes")
		require.Nil(t, token, "a refused caller gets no token")

		lock(t, s, "order-2", payload("other"))
	})
}

// TestStorage_AcceptsTheKeysTheKeeperBuilds pins that a storage takes every
// key shape the layers above it produce: the HTTP middleware and the gRPC
// interceptor build "idk:{service}:{key}", and a Keeper namespace prefixes
// "{namespace}:". A storage that refuses them fails every request routed
// through those layers.
func TestStorage_AcceptsTheKeysTheKeeperBuilds(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		s := b.open(t, longTTL)
		for _, key := range []string{
			"idk:POST:/api/v1/orders:550e8400-e29b-41d4-a716-446655440000",
			"idk:users.UserService:550e8400-e29b-41d4-a716-446655440000",
			"acme:invoice-1",
		} {
			t.Run(key, func(t *testing.T) {
				token := lock(t, s, key, payload("lock"))
				require.NoError(t, s.Complete(t.Context(), key, payload("done"), token))
				require.Equal(t, payload("done"), observe(t, s, key))
			})
		}
	})
}

// TestStorage_EmptyKeyIsRefused pins that no operation silently accepts an
// empty key, which would disable deduplication.
func TestStorage_EmptyKeyIsRefused(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		s := b.open(t, longTTL)
		ctx := t.Context()

		_, _, _, err := s.AttemptLock(ctx, "", payload("v"))
		require.ErrorIs(t, err, storages.ErrEmptyKey)
		_, _, _, err = s.AttemptLockWithTTL(ctx, "", payload("v"), shortTTL)
		require.ErrorIs(t, err, storages.ErrEmptyKey)
		require.ErrorIs(t, s.Complete(ctx, "", payload("v"), []byte("token")), storages.ErrEmptyKey)
		_, err = s.Steal(ctx, "", payload("a"), payload("b"))
		require.ErrorIs(t, err, storages.ErrEmptyKey)
		require.ErrorIs(t, s.Release(ctx, "", []byte("token")), storages.ErrEmptyKey)
		require.ErrorIs(t, s.Delete(ctx, ""), storages.ErrEmptyKey)
	})
}

// TestStorage_Complete covers every outcome of the compare-and-set behind
// Complete: the holder's token writes, a token of another lock and a key
// that vanished are refused without writing, and a nil token overwrites.
func TestStorage_Complete(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		t.Run("HolderTokenWrites", func(t *testing.T) {
			t.Parallel()
			s := b.open(t, longTTL)
			token := lock(t, s, "k", payload("lock"))

			require.NoError(t, s.Complete(t.Context(), "k", payload("done"), token))
			require.Equal(t, payload("done"), observe(t, s, "k"))
		})

		t.Run("ForeignTokenIsRefused", func(t *testing.T) {
			t.Parallel()
			s := b.open(t, longTTL)
			lock(t, s, "k", payload("lock"))
			// A well-formed token that belongs to another key's lock.
			foreign := lock(t, s, "other", payload("other-lock"))

			require.ErrorIs(t, s.Complete(t.Context(), "k", payload("done"), foreign), storages.ErrLockStolen)
			require.Equal(t, payload("lock"), observe(t, s, "k"), "a refused Complete must not write")
		})

		t.Run("StaleTokenIsRefused", func(t *testing.T) {
			t.Parallel()
			s := b.open(t, longTTL)
			stale := lock(t, s, "k", payload("lock"))
			_, err := s.Steal(t.Context(), "k", payload("lock"), payload("taken-over"))
			require.NoError(t, err)

			require.ErrorIs(t, s.Complete(t.Context(), "k", payload("done"), stale), storages.ErrLockStolen)
			require.Equal(t, payload("taken-over"), observe(t, s, "k"))
		})

		t.Run("VanishedKeyIsRefusedAndNotRecreated", func(t *testing.T) {
			t.Parallel()
			s := b.open(t, longTTL)
			token := lock(t, s, "k", payload("lock"))
			require.NoError(t, s.Delete(t.Context(), "k"))

			require.ErrorIs(t, s.Complete(t.Context(), "k", payload("done"), token), storages.ErrLockStolen)
			requireFree(t, s, "k")
		})

		t.Run("NilTokenOverwrites", func(t *testing.T) {
			t.Parallel()
			s := b.open(t, longTTL)
			lock(t, s, "k", payload("lock"))

			require.NoError(t, s.Complete(t.Context(), "k", payload("forced"), nil))
			require.Equal(t, payload("forced"), observe(t, s, "k"))
		})
	})
}

// TestStorage_Steal covers the orphan takeover: a matching expectation
// replaces the value and fences the previous holder, a stale one or a
// missing key is refused without writing.
func TestStorage_Steal(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		t.Run("MatchTakesOverAndFencesThePreviousHolder", func(t *testing.T) {
			t.Parallel()
			s := b.open(t, longTTL)
			ctx := t.Context()
			old := lock(t, s, "k", payload("orphan"))

			fresh, err := s.Steal(ctx, "k", payload("orphan"), payload("reclaimed"))
			require.NoError(t, err)
			require.NotEmpty(t, fresh)
			require.Equal(t, payload("reclaimed"), observe(t, s, "k"))

			require.ErrorIs(t, s.Complete(ctx, "k", payload("late"), old), storages.ErrLockStolen)
			require.ErrorIs(t, s.Release(ctx, "k", old), storages.ErrLockStolen)
			require.NoError(t, s.Complete(ctx, "k", payload("done"), fresh))
			require.Equal(t, payload("done"), observe(t, s, "k"))
		})

		t.Run("StaleExpectationIsRefused", func(t *testing.T) {
			t.Parallel()
			s := b.open(t, longTTL)
			lock(t, s, "k", payload("current"))

			_, err := s.Steal(t.Context(), "k", payload("stale"), payload("reclaimed"))
			require.ErrorIs(t, err, storages.ErrLockStolen)
			require.Equal(t, payload("current"), observe(t, s, "k"))
		})

		t.Run("MissingKeyIsRefusedAndNotCreated", func(t *testing.T) {
			t.Parallel()
			s := b.open(t, longTTL)

			_, err := s.Steal(t.Context(), "k", payload("orphan"), payload("reclaimed"))
			require.ErrorIs(t, err, storages.ErrLockStolen)
			requireFree(t, s, "k")
		})
	})
}

// TestStorage_Release covers the owner-only delete a failed request uses.
func TestStorage_Release(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		t.Run("HolderFreesTheKey", func(t *testing.T) {
			t.Parallel()
			s := b.open(t, longTTL)
			token := lock(t, s, "k", payload("lock"))

			require.NoError(t, s.Release(t.Context(), "k", token))
			requireFree(t, s, "k")
		})

		t.Run("ForeignTokenKeepsTheLock", func(t *testing.T) {
			t.Parallel()
			s := b.open(t, longTTL)
			lock(t, s, "k", payload("lock"))
			foreign := lock(t, s, "other", payload("other-lock"))

			require.ErrorIs(t, s.Release(t.Context(), "k", foreign), storages.ErrLockStolen)
			require.Equal(t, payload("lock"), observe(t, s, "k"))
		})

		t.Run("CompletedResultSurvivesTheHolder", func(t *testing.T) {
			t.Parallel()
			s := b.open(t, longTTL)
			token := lock(t, s, "k", payload("lock"))
			require.NoError(t, s.Complete(t.Context(), "k", payload("done"), token))

			require.ErrorIs(t, s.Release(t.Context(), "k", token), storages.ErrLockStolen)
			require.Equal(t, payload("done"), observe(t, s, "k"))
		})

		t.Run("MissingKeyIsRefused", func(t *testing.T) {
			t.Parallel()
			s := b.open(t, longTTL)
			token := lock(t, s, "k", payload("lock"))
			require.NoError(t, s.Delete(t.Context(), "k"))

			require.ErrorIs(t, s.Release(t.Context(), "k", token), storages.ErrLockStolen)
		})

		t.Run("NilTokenIsRefused", func(t *testing.T) {
			t.Parallel()
			s := b.open(t, longTTL)
			lock(t, s, "k", payload("lock"))

			require.ErrorIs(t, s.Release(t.Context(), "k", nil), storages.ErrMissingLockState)
			require.Equal(t, payload("lock"), observe(t, s, "k"))
		})
	})
}

// TestStorage_Delete pins the administrative delete: it removes whatever the
// key holds, and deleting a missing key is not an error.
func TestStorage_Delete(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		s := b.open(t, longTTL)
		ctx := t.Context()

		lock(t, s, "in-progress", payload("lock"))
		token := lock(t, s, "completed", payload("lock"))
		require.NoError(t, s.Complete(ctx, "completed", payload("done"), token))

		for _, key := range []string{"in-progress", "completed"} {
			require.NoError(t, s.Delete(ctx, key))
			requireFree(t, s, key)
		}
		require.NoError(t, s.Delete(ctx, "never-written"))
	})
}

// TestStorage_PerCallTTLExpiresOnTheServer pins AttemptLockWithTTL: the key
// ages out after the per-call TTL although the storage TTL is an hour, its
// sibling locked with the storage TTL does not, and the expired holder is
// refused afterwards rather than overwriting the next one.
func TestStorage_PerCallTTLExpiresOnTheServer(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		s := b.open(t, longTTL)
		ctx := t.Context()

		acquired, _, expired, err := s.AttemptLockWithTTL(ctx, "short", payload("first"), shortTTL)
		require.NoError(t, err)
		require.True(t, acquired)
		lock(t, s, "long", payload("sibling"))
		require.Equal(t, payload("first"), observe(t, s, "short"), "the lock must hold until its TTL elapses")

		next := awaitAcquire(t, s, "short", payload("second"))

		require.ErrorIs(t, s.Complete(ctx, "short", payload("late"), expired), storages.ErrLockStolen)
		require.ErrorIs(t, s.Release(ctx, "short", expired), storages.ErrLockStolen)
		require.Equal(t, payload("second"), observe(t, s, "short"))
		require.NoError(t, s.Complete(ctx, "short", payload("done"), next))
		require.Equal(t, payload("sibling"), observe(t, s, "long"), "a per-call TTL must not shorten other keys")
	})
}

// TestStorage_ConfiguredTTLExpiresOnTheServer pins the storage TTL: a lock
// taken without a per-call TTL ages out after it, and so does a completed
// result — Complete renews the key with the storage TTL, not the lock's.
func TestStorage_ConfiguredTTLExpiresOnTheServer(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		t.Run("Lock", func(t *testing.T) {
			t.Parallel()
			s := b.open(t, shortTTL)
			expired := lock(t, s, "k", payload("first"))

			awaitAcquire(t, s, "k", payload("second"))
			require.ErrorIs(t, s.Complete(t.Context(), "k", payload("late"), expired), storages.ErrLockStolen)
		})

		t.Run("CompletedResult", func(t *testing.T) {
			t.Parallel()
			s := b.open(t, shortTTL)
			// A lock TTL far beyond the storage TTL: the completed result
			// must still age out with the storage TTL.
			acquired, _, token, err := s.AttemptLockWithTTL(t.Context(), "k", payload("lock"), longTTL)
			require.NoError(t, err)
			require.True(t, acquired)
			require.NoError(t, s.Complete(t.Context(), "k", payload("done"), token))

			awaitAcquire(t, s, "k", payload("retry"))
		})
	})
}

// TestStorage_ConcurrentAttemptLockHasOneWinner races many callers for one
// key: exactly one acquires, and every other sees the winner's bytes.
func TestStorage_ConcurrentAttemptLockHasOneWinner(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		s := b.open(t, longTTL)

		type outcome struct {
			acquired bool
			existing []byte
			err      error
		}
		outcomes := make([]outcome, contenders)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range contenders {
			wg.Go(func() {
				<-start
				acquired, existing, _, err := s.AttemptLock(t.Context(), "contended", payload(fmt.Sprint("contender-", i)))
				outcomes[i] = outcome{acquired, existing, err}
			})
		}
		close(start)
		wg.Wait()

		winner := -1
		for i, o := range outcomes {
			require.NoError(t, o.err, "contender %d", i)
			if o.acquired {
				require.Equal(t, -1, winner, "contenders %d and %d both acquired the key", winner, i)
				winner = i
			}
		}
		require.NotEqual(t, -1, winner, "no contender acquired the key")
		want := payload(fmt.Sprint("contender-", winner))
		for i, o := range outcomes {
			if i != winner {
				require.Equal(t, want, o.existing, "contender %d saw another holder than the winner", i)
			}
		}
		require.Equal(t, want, observe(t, s, "contended"))
	})
}

// TestStorage_ConcurrentStealHasOneWinner races many reclaimers for one
// orphaned lock: the compare-and-set lets exactly one through.
func TestStorage_ConcurrentStealHasOneWinner(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		s := b.open(t, longTTL)
		lock(t, s, "orphan", payload("orphan"))

		var (
			wins   atomic.Int32
			winner atomic.Int32
			wg     sync.WaitGroup
		)
		errs := make([]error, contenders)
		start := make(chan struct{})
		for i := range contenders {
			wg.Go(func() {
				<-start
				_, err := s.Steal(t.Context(), "orphan", payload("orphan"), payload(fmt.Sprint("reclaimer-", i)))
				if err == nil {
					wins.Add(1)
					winner.Store(int32(i))
					return
				}
				errs[i] = err
			})
		}
		close(start)
		wg.Wait()

		require.EqualValues(t, 1, wins.Load(), "exactly one reclaimer must win")
		for i, err := range errs {
			if i != int(winner.Load()) {
				require.ErrorIs(t, err, storages.ErrLockStolen, "reclaimer %d", i)
			}
		}
		require.Equal(t, payload(fmt.Sprint("reclaimer-", winner.Load())), observe(t, s, "orphan"))
	})
}

// TestStorage_ConcurrentReleaseHasOneWinner releases one lock from many
// goroutines with the same token: one delete succeeds, the rest find the key
// gone.
func TestStorage_ConcurrentReleaseHasOneWinner(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		s := b.open(t, longTTL)
		token := lock(t, s, "k", payload("lock"))

		var (
			released atomic.Int32
			wg       sync.WaitGroup
		)
		errs := make([]error, contenders)
		start := make(chan struct{})
		for i := range contenders {
			wg.Go(func() {
				<-start
				if errs[i] = s.Release(t.Context(), "k", token); errs[i] == nil {
					released.Add(1)
				}
			})
		}
		close(start)
		wg.Wait()

		require.EqualValues(t, 1, released.Load(), "exactly one release must delete the key")
		for i, err := range errs {
			if err != nil {
				require.ErrorIs(t, err, storages.ErrLockStolen, "releaser %d", i)
			}
		}
		requireFree(t, s, "k")
	})
}

// TestStorage_LargeBinaryValues runs the whole lifecycle with random values
// of half a megabyte, the size class where a compare-and-set on the stored
// bytes is most expensive. It stays under the default NATS payload limit.
func TestStorage_LargeBinaryValues(t *testing.T) {
	t.Parallel()
	forEachBackend(t, func(t *testing.T, b backend) {
		s := b.open(t, longTTL)
		ctx := t.Context()
		big := func() []byte {
			v := make([]byte, 512<<10)
			_, _ = rand.Read(v)
			return v
		}
		first, reclaimed, done := big(), big(), big()

		token := lock(t, s, "k", first)
		require.Equal(t, first, observe(t, s, "k"))

		// A value that differs from the stored one only in its last byte.
		almost := append([]byte(nil), first...)
		almost[len(almost)-1] ^= 0xff
		_, err := s.Steal(ctx, "k", almost, reclaimed)
		require.ErrorIs(t, err, storages.ErrLockStolen)

		fresh, err := s.Steal(ctx, "k", first, reclaimed)
		require.NoError(t, err)
		require.ErrorIs(t, s.Complete(ctx, "k", done, token), storages.ErrLockStolen)
		require.NoError(t, s.Complete(ctx, "k", done, fresh))
		require.Equal(t, done, observe(t, s, "k"))
	})
}
