// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
	"iter"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter"
	"github.com/altessa-s/go-atlas/cache/probfilter/bloom"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/redistest"

	bloommemory "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages/memory"
	bloomredis "github.com/altessa-s/go-atlas/cache/probfilter/bloom/storages/redis"
	goredis "github.com/redis/go-redis/v9"
)

// fakeBloomModule emulates the RedisBloom BF.* commands the Redis Bloom
// storage sends, on a miniredis that does not ship the module. A filter is a
// string key holding its items one per line, so the storage's own RENAME
// promotes a staging filter like the real one.
//
// BF.RESERVE only ever runs inside a script, where a miniredis command
// handler cannot write keys: it records the key and the client hook creates
// it once the script returned. Inserts into the live filter, which run inside
// the journaling script, are not emulated.
//
// Membership lookups run inside a Lua script, which holds the miniredis lock,
// so a handler cannot read keys through the miniredis API there. The module
// therefore keeps a snapshot of every filter, refreshed by the client hook
// after each command, and a handler reads it whenever the lock is taken.
type fakeBloomModule struct {
	mr       *miniredis.Miniredis
	mu       sync.Mutex
	pending  []string
	snapshot map[string][]string
}

// newSharedBloomRedis returns a client of an in-process Redis emulating BF.*.
func newSharedBloomRedis(t *testing.T) (*goredis.Client, *miniredis.Miniredis) {
	t.Helper()
	client, mr := redistest.RedisClient(t)
	fake := &fakeBloomModule{mr: mr}
	srv := mr.Server()

	require.NoError(t, srv.Register("BF.RESERVE", func(c *server.Peer, _ string, args []string) {
		fake.mu.Lock()
		fake.pending = append(fake.pending, args[0])
		fake.mu.Unlock()
		c.WriteOK()
	}))
	require.NoError(t, srv.Register("BF.INSERT", func(c *server.Peer, _ string, args []string) {
		key := args[0]
		if !mr.Exists(key) {
			c.WriteError("ERR not found")
			return
		}
		held := fake.items(key)
		added := args[slices.Index(args, "ITEMS")+1:]
		c.WriteLen(len(added))
		for _, item := range added {
			if slices.Contains(held, item) {
				c.WriteInt(0)
				continue
			}
			held = append(held, item)
			c.WriteInt(1)
		}
		_ = mr.Set(key, strings.Join(held, "\n"))
	}))
	require.NoError(t, srv.Register("BF.EXISTS", func(c *server.Peer, _ string, args []string) {
		if slices.Contains(fake.lookup(args[0]), args[1]) {
			c.WriteInt(1)
			return
		}
		c.WriteInt(0)
	}))
	require.NoError(t, srv.Register("BF.INFO", func(c *server.Peer, _ string, args []string) {
		if !mr.Exists(args[0]) {
			c.WriteError("ERR not found")
			return
		}
		c.WriteLen(4)
		c.WriteBulk("Capacity")
		c.WriteInt(1000)
		c.WriteBulk("Number of items inserted")
		c.WriteInt(len(fake.items(args[0])))
	}))

	client.AddHook(fake)
	return client, mr
}

// items returns the items of the filter stored under key.
func (f *fakeBloomModule) items(key string) []string {
	value, err := f.mr.Get(key)
	if err != nil || value == "" {
		return nil
	}
	return strings.Split(value, "\n")
}

// lookup returns the items of the filter stored under key. Inside a script
// the miniredis lock is held by the script itself, so the snapshot taken after
// the previous command answers instead.
func (f *fakeBloomModule) lookup(key string) []string {
	if f.mr.TryLock() {
		f.mr.Unlock()
		return f.items(key)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.snapshot[key]
}

// materialize creates the filters reserved by the command that just returned
// and refreshes the snapshot of all filters.
func (f *fakeBloomModule) materialize() {
	f.mu.Lock()
	pending := f.pending
	f.pending = nil
	f.mu.Unlock()
	for _, key := range pending {
		if !f.mr.Exists(key) {
			_ = f.mr.Set(key, "")
		}
	}
	snapshot := make(map[string][]string)
	for _, key := range f.mr.Keys() {
		if f.mr.Type(key) == "string" {
			snapshot[key] = f.items(key)
		}
	}
	f.mu.Lock()
	f.snapshot = snapshot
	f.mu.Unlock()
}

func (f *fakeBloomModule) DialHook(next goredis.DialHook) goredis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) { return next(ctx, network, addr) }
}

func (f *fakeBloomModule) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		err := next(ctx, cmd)
		f.materialize()
		return err
	}
}

func (f *fakeBloomModule) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []goredis.Cmder) error {
		err := next(ctx, cmds)
		f.materialize()
		return err
	}
}

// scriptedSource is a revocation source whose next answers a test sets.
type scriptedSource struct {
	mu      sync.Mutex
	entries []string
	err     error
}

func (s *scriptedSource) set(entries ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries, s.err = entries, nil
}

func (s *scriptedSource) StreamValues(context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		s.mu.Lock()
		entries, err := slices.Clone(s.entries), s.err
		s.mu.Unlock()
		for _, entry := range entries {
			if !yield(entry, nil) {
				return
			}
		}
		if err != nil {
			yield("", err)
		}
	}
}

func (s *scriptedSource) Count(context.Context) (int64, error) { return -1, nil }

// sharedReplica returns the filter storage of one replica of the shared
// revocation filter and the source it syncs from.
func sharedReplica(client *goredis.Client, entries ...string) (*filterRevocationStorage, *scriptedSource) {
	source := &scriptedSource{entries: entries}
	return &filterRevocationStorage{
		filter: bloom.New(bloomredis.New(client, "oidc-revocation-shared")),
		loader: source,
	}, source
}

// holdRebuildLease starts a sync of a peer replica that stays inside its
// source, holding the shared rebuild lease, until the returned function is
// called; the sync then fails with result, or commits entries when result is
// nil. The returned function reports the sync's error.
func holdRebuildLease(t *testing.T, client *goredis.Client, result error, entries ...string) func() error {
	t.Helper()
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	peer := &filterRevocationStorage{
		filter: bloom.New(bloomredis.New(client, "oidc-revocation-shared")),
		loader: probfilter.DataLoaderFunc(func(context.Context) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				close(started)
				<-release
				for _, entry := range entries {
					if !yield(entry, nil) {
						return
					}
				}
				if result != nil {
					yield("", result)
				}
			}
		}),
	}
	go func() { done <- peer.Sync(context.WithoutCancel(t.Context())) }()
	<-started

	var once sync.Once
	finish := func() (err error) {
		once.Do(func() {
			close(release)
			err = <-done
		})
		return err
	}
	t.Cleanup(func() { _ = finish() })
	return finish
}

// sharedFilterProvider starts a provider on the shared filter with a sync
// schedule that never ticks during the test and a one-hour staleness bound.
func sharedFilterProvider(t *testing.T, idp *testIdP, client *goredis.Client, entries ...string) *Provider {
	t.Helper()
	storage, _ := sharedReplica(client, entries...)
	return idp.newProvider(t,
		WithRevocationStorage(storage), WithRevocationItemType(RevocationItemTypeJTI),
		WithRevocationSyncSchedule("@every 24h"), WithRevocationMaxStaleness(time.Hour))
}

// A replica that keeps losing the rebuild lease stays fresh for as long as
// some peer commits: the committed sync is read from the shared filter, and
// the replica's own clock is left alone.
func TestScheduledRevocationSync_PeerCommitKeepsLosingReplicaFresh(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	client, _ := newSharedBloomRedis(t)
	p := sharedFilterProvider(t, idp, client, "j1")
	peer, _ := sharedReplica(client, "j1", "j2")

	for range 3 {
		backdateRevocationSync(p, 2*time.Hour)
		p.revocation.sharedRevocationSync.Store(nil)
		own := p.revocation.lastRevocationSync.Load()
		require.ErrorIs(t, p.revocation.checkRevocationStaleness(), ErrRevocationStale)

		require.NoError(t, peer.Sync(t.Context()))
		finish := holdRebuildLease(t, client, nil, "j1", "j2")

		require.NoError(t, p.revocation.scheduledRevocationSync(t.Context()), "a rebuild in progress is skipped silently")
		require.Equal(t, own, p.revocation.lastRevocationSync.Load(), "losing the lease is not a sync of this replica")
		require.NoError(t, p.revocation.checkRevocationStaleness())
		require.True(t, jtiRevoked(t, p, idp, "j2"), "the peer's list is in force")
		require.NoError(t, finish())
	}
}

// A peer that holds the rebuild lease and then fails published nothing: the
// losing replica must not count it, and goes stale once the bound passes.
func TestScheduledRevocationSync_FailedPeerRebuildDoesNotRefresh(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	client, mr := newSharedBloomRedis(t)
	committed := time.Now()
	mr.SetTime(committed)
	p := sharedFilterProvider(t, idp, client, "j1")
	raw := idp.sign(t, idp.claims(map[string]any{"jti": "valid"}))
	_, err := p.ValidateToken(t.Context(), raw)
	require.NoError(t, err)
	// The only commit the shared filter ever sees is two hours old.
	mr.SetTime(committed.Add(2 * time.Hour))
	backdateRevocationSync(p, 2*time.Hour)

	sourceDown := errors.New("source down")
	for range 3 {
		finish := holdRebuildLease(t, client, sourceDown)
		require.NoError(t, p.revocation.scheduledRevocationSync(t.Context()))
		require.ErrorIs(t, finish(), sourceDown)

		require.ErrorIs(t, p.revocation.checkRevocationStaleness(), ErrRevocationStale)
		_, err = p.ValidateToken(t.Context(), raw)
		require.ErrorIs(t, err, ErrRevocationCheck)
		require.ErrorIs(t, err, ErrRevocationStale)
	}

	// The replica's own next sync gets the lease and restores freshness.
	require.NoError(t, p.revocation.scheduledRevocationSync(t.Context()))
	_, err = p.ValidateToken(t.Context(), raw)
	require.NoError(t, err)
}

// The committed sync time is an instant of the Redis clock. One that runs
// ahead of the local clock must not push the cached time into the future.
func TestScheduledRevocationSync_SharedTimeAheadOfLocalClockIsClamped(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	client, mr := newSharedBloomRedis(t)
	p := sharedFilterProvider(t, idp, client, "j1")
	peer, _ := sharedReplica(client, "j1")

	mr.SetTime(time.Now().Add(10 * time.Hour))
	require.NoError(t, peer.Sync(t.Context()))
	finish := holdRebuildLease(t, client, nil, "j1")
	backdateRevocationSync(p, 2*time.Hour)
	require.NoError(t, p.revocation.scheduledRevocationSync(t.Context()))
	require.NoError(t, finish())

	require.False(t, p.revocation.sharedRevocationSync.Load().observedAt.After(time.Now()))
	require.NoError(t, p.revocation.checkRevocationStaleness())
}

// A storage that reports no committed sync behaves as before, minus the peer
// stamp: a rebuild in progress does not refresh it.
func TestScheduledRevocationSync_StorageWithoutReporterIsNotRefreshedByPeer(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	storage := &inProgressStorage{}
	p := idp.newProvider(t, WithRevocationStorage(storage),
		WithRevocationSyncSchedule("@every 24h"), WithRevocationMaxStaleness(time.Hour))
	backdateRevocationSync(p, 2*time.Hour)

	storage.busy = true
	require.NoError(t, p.revocation.scheduledRevocationSync(t.Context()))
	require.ErrorIs(t, p.revocation.checkRevocationStaleness(), ErrRevocationStale)

	storage.busy = false
	require.NoError(t, p.revocation.scheduledRevocationSync(t.Context()))
	require.NoError(t, p.revocation.checkRevocationStaleness())
}

// inProgressStorage is a custom storage whose Sync reports a peer's rebuild.
type inProgressStorage struct{ busy bool }

func (*inProgressStorage) IsRevoked(context.Context, string) (bool, error) { return false, nil }

func (*inProgressStorage) MarkRevoked(context.Context, string, time.Duration) error { return nil }

func (s *inProgressStorage) Sync(context.Context) error {
	if s.busy {
		return probfilter.ErrRebuildInProgress
	}
	return nil
}

// A replica that synced the shared filter while the list was empty must not
// take its own empty result for the state of the filter: a peer published a
// list since, and an erroneous empty response would un-revoke all of it.
func TestRevocationSync_EmptyResultKeepsListPublishedByPeer(t *testing.T) {
	t.Parallel()

	client, _ := newSharedBloomRedis(t)
	a, _ := sharedReplica(client)
	b, _ := sharedReplica(client, "j1", "j2")

	require.NoError(t, a.Sync(t.Context()), "an empty list on an empty shared filter is just an empty list")
	require.NoError(t, b.Sync(t.Context()))

	require.ErrorIs(t, a.Sync(t.Context()), ErrRevocationLoadFailed)
	for _, jti := range []string{"j1", "j2"} {
		revoked, err := a.IsRevoked(t.Context(), jti)
		require.NoError(t, err)
		require.True(t, revoked)
	}

	a.allowEmpty = true
	require.NoError(t, a.Sync(t.Context()))
	revoked, err := b.IsRevoked(t.Context(), "j1")
	require.NoError(t, err)
	require.False(t, revoked, "allowEmpty makes the empty list authoritative for every replica")
}

// A single-process in-memory filter accepts a legitimately empty first sync
// and rejects an empty result once entries were loaded.
func TestRevocationSync_MemoryFilterEmptyGuard(t *testing.T) {
	t.Parallel()

	source := &scriptedSource{}
	storage := &filterRevocationStorage{filter: bloom.New(bloommemory.New()), loader: source}

	require.NoError(t, storage.Sync(t.Context()))
	require.NoError(t, storage.Sync(t.Context()))

	source.set("j1")
	require.NoError(t, storage.Sync(t.Context()))

	source.set()
	require.ErrorIs(t, storage.Sync(t.Context()), ErrRevocationLoadFailed)
	revoked, err := storage.IsRevoked(t.Context(), "j1")
	require.NoError(t, err)
	require.True(t, revoked)
}

// The filter storage reports the filter's last committed rebuild and its age
// on the filter's own clock: the Redis time of a commit by any replica, the
// local rebuild time of a memory filter.
func TestFilterRevocationStorage_LastCommittedSync(t *testing.T) {
	t.Parallel()

	t.Run("shared filter", func(t *testing.T) {
		t.Parallel()
		client, mr := newSharedBloomRedis(t)
		a, _ := sharedReplica(client, "j1")
		b, _ := sharedReplica(client)

		_, _, known, err := b.LastCommittedSync(t.Context())
		require.NoError(t, err)
		require.False(t, known)

		at := time.UnixMilli(1_700_000_000_000)
		mr.SetTime(at)
		require.NoError(t, a.Sync(t.Context()))
		mr.SetTime(at.Add(90 * time.Second))
		committed, age, known, err := b.LastCommittedSync(t.Context())
		require.NoError(t, err)
		require.True(t, known)
		require.True(t, at.Equal(committed))
		require.Equal(t, 90*time.Second, age, "the age is a difference within the Redis clock")
	})

	t.Run("memory filter", func(t *testing.T) {
		t.Parallel()
		storage := &filterRevocationStorage{filter: bloom.New(bloommemory.New()), loader: &scriptedSource{entries: []string{"j1"}}}
		before := time.Now()
		require.NoError(t, storage.Sync(t.Context()))
		committed, age, known, err := storage.LastCommittedSync(t.Context())
		require.NoError(t, err)
		require.True(t, known)
		require.False(t, committed.Before(before))
		require.Less(t, age, time.Minute)
	})

	t.Run("filter without commit times", func(t *testing.T) {
		t.Parallel()
		storage := &filterRevocationStorage{filter: newRebuildableFilter()}
		_, _, known, err := storage.LastCommittedSync(t.Context())
		require.NoError(t, err)
		require.False(t, known)
	})
}
