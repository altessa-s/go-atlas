// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/encoding/keys"
	"github.com/altessa-s/go-atlas/coord/leadelect/providers"
	"github.com/altessa-s/go-atlas/internal/nats/natskvlease"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
)

// electionCtxKey marks the election context, so resignProbeKV can tell the
// resign call apart: resign runs on a context of its own.
type electionCtxKey struct{}

// resignProbeKV grants every acquisition and records what the provider reports
// at the moment it releases the key.
type resignProbeKV struct {
	jetstream.KeyValue

	p       *Provider
	updates chan jetstream.KeyValueEntry

	resigns            atomic.Int64
	leaderDuringResign atomic.Bool
	fenceDuringResign  atomic.Uint64
}

func (k *resignProbeKV) Create(context.Context, string, []byte, ...jetstream.KVCreateOpt) (uint64, error) {
	return 7, nil
}

func (k *resignProbeKV) Get(ctx context.Context, _ string) (jetstream.KeyValueEntry, error) {
	if ctx.Value(electionCtxKey{}) == nil {
		k.resigns.Add(1)
		k.leaderDuringResign.Store(k.p.isLeader.Load())
		k.fenceDuringResign.Store(k.p.Fence())
	}
	return nil, jetstream.ErrKeyNotFound
}

func (k *resignProbeKV) Watch(context.Context, string, ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	return &fakeWatcher{updates: k.updates}, nil
}

func TestCamping_ContextCancelClearsLeadershipBeforeReleasingKey(t *testing.T) {
	t.Parallel()

	kv := &resignProbeKV{updates: make(chan jetstream.KeyValueEntry, 1)}
	p := fakeProvider(kv, providers.Config{Key: "k", NodeId: "n", TTL: time.Hour})
	kv.p = p

	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), electionCtxKey{}, true))
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.camping(ctx)
	}()

	kv.updates <- nil // end of the initial values: the node acquires
	testhelpers.WaitFor(t, 5*time.Second, p.IsLeader, "the node did not become leader")
	require.EqualValues(t, 7, p.Fence())

	cancel()
	<-done

	require.EqualValues(t, 1, kv.resigns.Load(), "the key must be released exactly once")
	require.False(t, kv.leaderDuringResign.Load(), "leadership must be cleared before the key is released")
	require.Zero(t, kv.fenceDuringResign.Load(), "no fencing token may be handed out while the key is being released")

	require.False(t, p.IsLeader())
	require.Zero(t, p.Fence())
	require.False(t, p.IsRunning(), "an election ended by its context must not report itself running")

	p.notifyMu.Lock()
	defer p.notifyMu.Unlock()
	require.False(t, p.notifyLeader, "the loss must be published")
	require.True(t, p.notifyStopped, "the stop must be published")
}

func TestProvider_StartContextCancelHandsOverWithoutSplitBrain(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	newProvider := func() *Provider {
		p, err := New(t.Context(), natstest.ConnectNATS(t, ns), WithBucket("handover-on-cancel"))
		require.NoError(t, err)
		return p
	}
	a, b := newProvider(), newProvider()

	lostCh := make(chan struct{}, 1)
	stopCh := make(chan struct{}, 1)
	ctxA, cancelA := context.WithCancel(t.Context())
	defer cancelA()
	require.NoError(t, a.Start(ctxA, providers.Config{Key: "k", NodeId: "a", TTL: 30 * time.Second, LostCh: lostCh, StopCh: stopCh}))
	testhelpers.WaitFor(t, 10*time.Second, a.IsLeader, "node a did not become leader")

	require.NoError(t, b.Start(t.Context(), providers.Config{Key: "k", NodeId: "b", TTL: 30 * time.Second}))
	t.Cleanup(func() { _ = b.Stop(context.WithoutCancel(t.Context())) }) //nolint:errcheck // best-effort teardown

	cancelA()
	testhelpers.WaitFor(t, 10*time.Second, b.IsLeader, "node b did not take over the released key")

	require.False(t, a.IsLeader(), "two nodes report leadership after node a resigned")
	require.Zero(t, a.Fence())

	for _, ch := range []struct {
		name string
		ch   <-chan struct{}
	}{{"lost", lostCh}, {"stopped", stopCh}} {
		select {
		case <-ch.ch:
		case <-time.After(5 * time.Second):
			require.FailNowf(t, "notification not delivered", "%s", ch.name)
		}
	}
	require.False(t, a.IsRunning())

	// Stop still pairs with Start: it joins the goroutines and allows a restart.
	require.NoError(t, a.Stop(t.Context()))
	require.NoError(t, a.Start(t.Context(), providers.Config{Key: "k", NodeId: "a", TTL: 30 * time.Second}))
	require.True(t, a.IsRunning())
	require.NoError(t, a.Stop(t.Context()))
}

func TestProvider_SameNodeIDHasOneLeader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	ns := natstest.StartNATSServer(t)
	const ratio = 0.15
	cfg := providers.Config{Key: "election", NodeId: "cluster:node", TTL: time.Second}
	start := func() *Provider {
		p, err := New(ctx, natstest.ConnectNATS(t, ns), WithBucket("same-id"), WithRenewRatio(ratio))
		require.NoError(t, err)
		require.NoError(t, p.Start(ctx, cfg))
		t.Cleanup(func() { _ = p.Stop(context.WithoutCancel(ctx)) })
		return p
	}
	a := start()
	require.Eventually(t, a.IsLeader, 5*time.Second, 10*time.Millisecond)
	b := start()
	require.Never(t, func() bool { return a.IsLeader() && b.IsLeader() },
		2*renewIntervalFor(cfg.TTL, ratio), 10*time.Millisecond)
	require.True(t, a.IsLeader())
	id, err := b.LeaderId(ctx)
	require.NoError(t, err)
	require.Equal(t, cfg.NodeId, id)
	require.NoError(t, b.Stop(ctx))
	require.True(t, a.IsLeader())
}

func TestProvider_StaleResignPreservesSuccessorWithSameNodeID(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	ns := natstest.StartNATSServer(t)
	nc := natstest.ConnectNATS(t, ns)
	const bucket = "stale-resign"
	a, err := New(ctx, nc, WithBucket(bucket))
	require.NoError(t, err)
	b, err := New(ctx, nc, WithBucket(bucket))
	require.NoError(t, err)
	cfg := providers.Config{Key: "election", NodeId: "cluster:node", TTL: time.Second}
	require.NoError(t, a.Start(ctx, cfg))
	t.Cleanup(func() { _ = a.Stop(context.WithoutCancel(ctx)) })
	require.Eventually(t, a.IsLeader, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, a.Stop(ctx))

	require.NoError(t, b.Start(ctx, cfg))
	t.Cleanup(func() { _ = b.Stop(context.WithoutCancel(ctx)) })
	require.Eventually(t, b.IsLeader, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, a.resign(ctx))
	_, err = b.kvOps.Get(ctx, cfg.Key)
	require.NoError(t, err)
}

type successorOnDeleteKV struct {
	jetstream.KeyValue
	successor string
}

func (kv successorOnDeleteKV) Delete(ctx context.Context, key string, opts ...jetstream.KVDeleteOpt) error {
	if _, err := kv.Put(ctx, key, []byte(kv.successor)); err != nil {
		return err
	}
	return kv.KeyValue.Delete(ctx, key, opts...)
}

func TestResign_RevisionCheckPreservesSuccessor(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	ns := natstest.StartNATSServer(t)
	p, err := New(ctx, natstest.ConnectNATS(t, ns), WithBucket("resign-check"))
	require.NoError(t, err)
	next := keys.Compose("next:node", "token")
	p.kvOps = natskvlease.NewKVOps(successorOnDeleteKV{KeyValue: p.kv, successor: next}, nil)
	cfg := providers.Config{Key: "election", NodeId: "node", TTL: time.Second}
	require.NoError(t, p.Start(ctx, cfg))
	t.Cleanup(func() { _ = p.Stop(context.WithoutCancel(ctx)) })
	require.Eventually(t, p.IsLeader, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, p.Stop(ctx))

	entry, err := p.kv.Get(ctx, cfg.Key)
	require.NoError(t, err)
	require.Equal(t, next, string(entry.Value()))
}
