// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/leadelect/providers"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	lenats "github.com/altessa-s/go-atlas/data/leadelect/providers/nats"
)

// leadingProvider returns a started provider that already holds the lease.
func leadingProvider(tb testing.TB, bucket string) (*lenats.Provider, *testhelpers.TestCollector, func()) {
	tb.Helper()

	ns := testhelpers.StartNATSServer(tb)
	nc := testhelpers.ConnectNATS(tb, ns)
	tc := testhelpers.NewTestCollector()

	prov, err := lenats.New(tb.Context(), nc, lenats.WithBucket(bucket), lenats.WithCollector(tc))
	require.NoError(tb, err)

	require.NoError(tb, prov.Start(tb.Context(), providers.Config{
		Key:      bucket,
		TTL:      2 * time.Second,
		NodeId:   "node-1",
		LostCh:   make(chan struct{}, 1),
		BecameCh: make(chan struct{}, 1),
		StopCh:   make(chan struct{}, 1),
	}))
	require.Eventually(tb, prov.IsLeader, 3*time.Second, 50*time.Millisecond, "should acquire leadership")

	killBroker := func() {
		ns.Shutdown()
		ns.WaitForShutdown()
	}
	return prov, tc, killBroker
}

// TestProvider_Stop_HonorsContextDeadline pins that the caller's shutdown budget
// bounds Stop. The resignation on the way out of the camping loop needs a
// context of its own, and an unbounded one let a dead broker hold shutdown for
// the NATS driver's internal timeout regardless of what the caller asked for.
func TestProvider_Stop_HonorsContextDeadline(t *testing.T) {
	t.Parallel()

	prov, _, killBroker := leadingProvider(t, "lifecycle-stopctx")
	killBroker()

	stopCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_ = prov.Stop(stopCtx)

	require.Less(t, time.Since(start), 700*time.Millisecond, "Stop must not exceed its context deadline")
}

// TestProvider_Stop_BoundedWithoutDeadline pins that a caller who supplies no
// deadline still gets a bounded shutdown rather than the driver's own timeout.
func TestProvider_Stop_BoundedWithoutDeadline(t *testing.T) {
	t.Parallel()

	prov, _, killBroker := leadingProvider(t, "lifecycle-stopnodl")
	killBroker()

	start := time.Now()
	_ = prov.Stop(context.Background())

	require.Less(t, time.Since(start), 3*time.Second, "Stop must be bounded by the default resign timeout")
}

// TestProvider_Stop_ClearsLeaderGauge pins that a stopped provider stops
// reporting itself as leader.
func TestProvider_Stop_ClearsLeaderGauge(t *testing.T) {
	t.Parallel()

	prov, tc, _ := leadingProvider(t, "lifecycle-gauge")

	const gauge = "test_nats_leader_election_is_leader"
	require.Equal(t, float64(1), testhelpers.GetGaugeValue(t, tc, gauge), "gauge should report leadership")

	require.NoError(t, prov.Stop(t.Context()))
	require.Equal(t, float64(0), testhelpers.GetGaugeValue(t, tc, gauge), "gauge must be cleared on Stop")
}

// TestProvider_Start_RejectedConfigLeavesProviderUsable pins that validation
// runs before the provider claims the running state. Claiming first left a
// rejected Start with isRunning=true and a nil configuration: the provider was
// permanently unstartable and NodeId dereferenced nil.
func TestProvider_Start_RejectedConfigLeavesProviderUsable(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	nc := testhelpers.ConnectNATS(t, ns)

	prov, err := lenats.New(t.Context(), nc, lenats.WithBucket("lifecycle-badstart"))
	require.NoError(t, err)

	require.Error(t, prov.Start(t.Context(), providers.Config{Key: "", TTL: 2 * time.Second, NodeId: "n1"}),
		"Start must reject an empty key")

	require.False(t, prov.IsRunning(), "a rejected Start must not leave the provider running")
	require.NotPanics(t, func() { _ = prov.NodeId() }, "NodeId must tolerate an unpublished configuration")

	require.NoError(t, prov.Start(t.Context(), providers.Config{
		Key:      "lifecycle-badstart",
		TTL:      2 * time.Second,
		NodeId:   "n1",
		LostCh:   make(chan struct{}, 1),
		BecameCh: make(chan struct{}, 1),
		StopCh:   make(chan struct{}, 1),
	}), "a rejected Start must not make the provider unrecoverable")
	require.NoError(t, prov.Stop(t.Context()))
}

// TestProvider_New_ReconcilesExistingBucketTTL pins that adopting a bucket that
// predates the provider does not silently produce leases that never expire.
func TestProvider_New_ReconcilesExistingBucketTTL(t *testing.T) {
	t.Parallel()

	ns := testhelpers.StartNATSServer(t)
	nc := testhelpers.ConnectNATS(t, ns)
	ctx := t.Context()

	js, err := jetstream.New(nc)
	require.NoError(t, err)

	const bucket = "lifecycle-preexisting"
	_, err = js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  bucket,
		Storage: jetstream.MemoryStorage,
		// TTL deliberately omitted: keys would never expire.
	})
	require.NoError(t, err)

	_, err = lenats.New(ctx, nc, lenats.WithBucket(bucket))
	require.NoError(t, err)

	kv, err := js.KeyValue(ctx, bucket)
	require.NoError(t, err)
	status, err := kv.Status(ctx)
	require.NoError(t, err)

	require.Equal(t, lenats.DefaultBucketKeysTTL, status.TTL(),
		"an adopted bucket must be reconciled to the lease TTL, otherwise the lease never expires")
}
