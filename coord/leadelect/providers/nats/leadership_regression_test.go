// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/leadelect/providers"
	"github.com/altessa-s/go-atlas/internal/nats/natskvlease"
	"github.com/altessa-s/go-atlas/internal/testhelpers"
)

var errKVDown = errors.New("kv unavailable")

// fakeKV is a jetstream.KeyValue whose election calls are scripted. Methods
// that are not overridden panic through the nil embedded interface.
type fakeKV struct {
	jetstream.KeyValue

	createErr error
	getErr    error

	watchFailures atomic.Int64 // Watch fails while positive
	watchCalls    atomic.Int64

	mu            sync.Mutex
	createCalledE time.Time
}

func (f *fakeKV) Create(context.Context, string, []byte, ...jetstream.KVCreateOpt) (uint64, error) {
	f.mu.Lock()
	f.createCalledE = time.Now()
	f.mu.Unlock()
	if f.createErr != nil {
		return 0, f.createErr
	}
	return 1, nil
}

func (f *fakeKV) createCalled() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.createCalledE
}

func (f *fakeKV) Get(context.Context, string) (jetstream.KeyValueEntry, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return nil, jetstream.ErrKeyNotFound
}

func (f *fakeKV) Watch(context.Context, string, ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	f.watchCalls.Add(1)
	if f.watchFailures.Add(-1) >= 0 {
		return nil, errKVDown
	}
	return &fakeWatcher{updates: make(chan jetstream.KeyValueEntry)}, nil
}

type fakeWatcher struct {
	updates chan jetstream.KeyValueEntry
}

func (w *fakeWatcher) Updates() <-chan jetstream.KeyValueEntry { return w.updates }
func (w *fakeWatcher) Stop() error                             { return nil }

// fakeProvider builds a Provider over kv without a NATS connection.
func fakeProvider(kv jetstream.KeyValue, cfg providers.Config) *Provider {
	opts := newOptions()
	p := &Provider{
		opts:           opts,
		metrics:        newNatsLeaderMetrics(opts.meter),
		kvOps:          natskvlease.NewKVOps(kv, slog.New(slog.DiscardHandler)),
		notificationCh: make(chan struct{}, 1),
	}
	p.isRunning.Store(true)
	p.providerConfig.Store(&cfg)
	return p
}

func newCampaign(t *testing.T, p *Provider) *campaign {
	t.Helper()
	return &campaign{p: p, ctx: t.Context(), logger: slog.New(slog.DiscardHandler), nodeID: "n"}
}

func TestCampaign_TransientErrorKeepsLeadership(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(c *campaign)
	}{
		{name: "acquire", run: func(c *campaign) { c.acquire() }},
		{name: "renew", run: func(c *campaign) { c.renew() }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			kv := &fakeKV{createErr: errKVDown, getErr: errKVDown}
			p := fakeProvider(kv, providers.Config{Key: "k", NodeId: "n", TTL: time.Hour})
			now := time.Now()
			p.lastRenewOK.Store(&now)
			p.isLeader.Store(true)
			p.notifyLeader = true

			tc.run(newCampaign(t, p))

			require.True(t, p.IsLeader(), "a transient KV error must not drop a lease that is still fresh")
			require.True(t, p.notifyLeader, "a transient KV error must not publish a lost event")
		})
	}
}

func TestCampaign_ErrorAfterLeaseLapsedPublishesLoss(t *testing.T) {
	t.Parallel()

	kv := &fakeKV{createErr: errKVDown, getErr: errKVDown}
	p := fakeProvider(kv, providers.Config{Key: "k", NodeId: "n", TTL: time.Second})
	old := time.Now().Add(-2 * time.Second)
	p.lastRenewOK.Store(&old)
	p.isLeader.Store(true)
	p.notifyLeader = true

	newCampaign(t, p).renew()

	require.False(t, p.isLeader.Load())
	require.False(t, p.notifyLeader, "a lease past its freshness bound must publish the loss")
}

func TestCampaign_LeaseStampedBeforeKVCall(t *testing.T) {
	t.Parallel()

	kv := &fakeKV{}
	p := fakeProvider(kv, providers.Config{Key: "k", NodeId: "n", TTL: time.Hour})

	newCampaign(t, p).acquire()

	require.True(t, p.IsLeader())
	require.False(t, p.lastRenewOK.Load().After(kv.createCalled()),
		"the lease instant must not be later than the write the server counts the TTL from")
}

func TestCamping_WatchFailureFallsBackToTicker(t *testing.T) {
	t.Parallel()

	kv := &fakeKV{}
	kv.watchFailures.Store(2)
	p := fakeProvider(kv, providers.Config{Key: "k", NodeId: "n", TTL: 100 * time.Millisecond})

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.camping(ctx)
	}()

	testhelpers.WaitFor(t, 5*time.Second, p.IsLeader, "a failed watch must not stop the node from acquiring leadership")
	testhelpers.WaitFor(t, 5*time.Second, func() bool { return kv.watchCalls.Load() >= 3 }, "the watch must be retried until it succeeds")

	cancel()
	<-done
}

type closedWatchKV struct {
	fakeKV
	attempts int
}

func (kv *closedWatchKV) Watch(ctx context.Context, key string, opts ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	if kv.watchCalls.Load() != 0 {
		return kv.fakeKV.Watch(ctx, key, opts...)
	}
	kv.watchCalls.Add(1)
	updates := make(chan jetstream.KeyValueEntry, 1)
	updates <- nil // initial snapshot marker
	close(updates)
	return &fakeWatcher{updates: updates}, nil
}

func (kv *closedWatchKV) Create(ctx context.Context, _ string, _ []byte, _ ...jetstream.KVCreateOpt) (uint64, error) {
	kv.attempts++
	if kv.attempts >= 10 {
		// Bound the old busy loop so the virtual clock can advance.
		<-ctx.Done()
	}
	return 0, errKVDown
}

func TestCamping_ClosedWatcherFallsBackToTicker(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		kv := &closedWatchKV{}
		const ttl = time.Second
		p := fakeProvider(kv, providers.Config{Key: "k", NodeId: "n", TTL: ttl})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go p.camping(ctx)

		time.Sleep(2 * renewIntervalFor(ttl, p.opts.renewRatio))
		synctest.Wait()
		require.LessOrEqual(t, kv.attempts, 3)
		require.EqualValues(t, 2, kv.watchCalls.Load())
	})
}

func TestStart_RejectsRenewIntervalRoundedToZero(t *testing.T) {
	t.Parallel()
	p := fakeProvider(&fakeKV{}, providers.Config{})
	p.opts.renewRatio = 1e-12
	err := p.Start(t.Context(), providers.Config{Key: "k", NodeId: "n", TTL: 10 * time.Second})
	if err == nil {
		require.NoError(t, p.Stop(t.Context()))
	}
	require.Error(t, err)
}

func TestNotifications_SlowConsumerEndsOnLatestState(t *testing.T) {
	t.Parallel()

	flap := func(p *Provider, times int) {
		for range times {
			p.setLeader(false)
			p.setLeader(true)
		}
	}
	// Intermediate edges coalesce, but a term the consumer was told about is
	// always closed by a "lost", and the term in effect is announced anew.
	tests := []struct {
		name   string
		pileUp func(p *Provider)
		want   []string
	}{
		{name: "ends lost", pileUp: func(p *Provider) { flap(p, 5); p.setLeader(false) }, want: []string{"became", "lost"}},
		{name: "one flap ends leader", pileUp: func(p *Provider) { flap(p, 1) }, want: []string{"became", "lost", "became"}},
		{name: "many flaps end leader", pileUp: func(p *Provider) { flap(p, 5) }, want: []string{"became", "lost", "became"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				becameCh := make(chan struct{})
				lostCh := make(chan struct{})
				cfg := providers.Config{Key: "k", NodeId: "n", TTL: time.Hour, BecameCh: becameCh, LostCh: lostCh}
				p := fakeProvider(&fakeKV{}, cfg)

				handlerDone := make(chan struct{})
				go func() {
					defer close(handlerDone)
					p.notificationHandler(t.Context(), &cfg)
				}()

				// The handler picks up the first transition and blocks
				// delivering it, because nobody reads yet.
				p.setLeader(true)
				synctest.Wait()

				// More transitions than the handler can hold pile up behind it.
				tc.pileUp(p)

				// Closing the queue makes the handler deliver what is pending
				// and exit. The channels are unbuffered, so once it has exited
				// the consumer has received every notification.
				close(p.notificationCh)

				var got []string
				for delivering := true; delivering; {
					select {
					case <-becameCh:
						got = append(got, "became")
					case <-lostCh:
						got = append(got, "lost")
					case <-handlerDone:
						delivering = false
					}
				}
				require.Equal(t, tc.want, got)
			})
		})
	}
}

func TestNotifications_FlapDeliversEveryEdgeToAFastConsumer(t *testing.T) {
	t.Parallel()

	becameCh := make(chan struct{})
	lostCh := make(chan struct{})
	cfg := providers.Config{Key: "k", NodeId: "n", TTL: time.Hour, BecameCh: becameCh, LostCh: lostCh}
	p := fakeProvider(&fakeKV{}, cfg)

	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		p.notificationHandler(t.Context(), &cfg)
	}()

	// A consumer that keeps up sees each edge of leader -> lost -> leader.
	for _, step := range []struct {
		leader bool
		ch     <-chan struct{}
	}{{true, becameCh}, {false, lostCh}, {true, becameCh}} {
		p.setLeader(step.leader)
		select {
		case <-step.ch:
		case <-time.After(5 * time.Second):
			require.FailNowf(t, "transition not delivered", "leader=%t", step.leader)
		}
	}

	close(p.notificationCh)
	<-handlerDone
}
