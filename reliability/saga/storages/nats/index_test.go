// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/reliability/saga"

	natsstore "github.com/altessa-s/go-atlas/reliability/saga/storages/nats"
)

const testBucket = "saga_index_test"

// newStoreWithKV returns a Store and the raw bucket behind it.
func newStoreWithKV(t *testing.T) (*natsstore.Store, jetstream.KeyValue) {
	t.Helper()
	s, kv, _ := newStoreWithConn(t)
	return s, kv
}

// newStoreWithConn also returns the client connection, to observe the requests.
func newStoreWithConn(t *testing.T) (*natsstore.Store, jetstream.KeyValue, *nats.Conn) {
	t.Helper()
	ns := natstest.StartNATSServer(t)
	nc, js := natstest.ConnectJetStream(t, ns)
	s, err := natsstore.New(t.Context(), js, natsstore.WithBucket(testBucket))
	require.NoError(t, err)
	kv, err := js.KeyValue(t.Context(), testBucket)
	require.NoError(t, err)
	return s, kv, nc
}

func indexKeys(t *testing.T, kv jetstream.KeyValue) []string {
	t.Helper()
	keys, err := kv.Keys(t.Context())
	if err != nil {
		require.ErrorIs(t, err, jetstream.ErrNoKeysFound)
	}
	var out []string
	for _, k := range keys {
		if strings.HasPrefix(k, "=rec.") {
			out = append(out, strings.TrimPrefix(k, "=rec."))
		}
	}
	slices.Sort(out)
	return out
}

func TestIndexTracksLiveInstancesOnly(t *testing.T) {
	t.Parallel()
	s, kv := newStoreWithKV(t)
	ctx := t.Context()

	live := &saga.Instance{ID: "live", Definition: "d", Status: saga.StatusRunning}
	done := &saga.Instance{ID: "done", Definition: "d", Status: saga.StatusCompleted}
	require.NoError(t, s.Create(ctx, live))
	require.NoError(t, s.Create(ctx, done))
	require.Equal(t, []string{"live"}, indexKeys(t, kv))

	live.Status = saga.StatusCompensating
	require.NoError(t, s.Update(ctx, live))
	require.Equal(t, []string{"live"}, indexKeys(t, kv))

	live.Status = saga.StatusCompleted
	require.NoError(t, s.Update(ctx, live))
	require.Empty(t, indexKeys(t, kv), "the terminal write removes the index key")

	other := &saga.Instance{ID: "other", Definition: "d", Status: saga.StatusRunning}
	require.NoError(t, s.Create(ctx, other))
	require.NoError(t, s.Delete(ctx, "other"))
	require.Empty(t, indexKeys(t, kv))
}

func TestFetchRecoverableIgnoresTerminalBacklog(t *testing.T) {
	t.Parallel()
	s, kv, nc := newStoreWithConn(t)
	ctx := t.Context()

	for i := range 200 {
		require.NoError(t, s.Create(ctx, &saga.Instance{ID: fmt.Sprintf("t-%03d", i), Definition: "d", Status: saga.StatusCompleted}))
	}
	require.NoError(t, s.Create(ctx, &saga.Instance{ID: "z-live", Definition: "d", Status: saga.StatusCompensating}))
	require.Equal(t, []string{"z-live"}, indexKeys(t, kv), "terminal instances are never indexed")

	// Record every JetStream request (key reads carry the key in the subject or
	// the payload) made by the scan.
	var mu sync.Mutex
	var requests []string
	sub, err := nc.Subscribe("$JS.API.>", func(m *nats.Msg) {
		mu.Lock()
		requests = append(requests, m.Subject+" "+string(m.Data))
		mu.Unlock()
	})
	require.NoError(t, err)
	require.NoError(t, nc.Flush())

	rec, err := s.FetchRecoverable(ctx, "d", time.Now(), 10)
	require.NoError(t, err)
	require.Len(t, rec, 1)
	require.Equal(t, "z-live", rec[0].ID)

	require.NoError(t, nc.Flush())
	require.NoError(t, sub.Drain())
	mu.Lock()
	defer mu.Unlock()
	joined := strings.Join(requests, "\n")
	require.Contains(t, joined, "z-live", "the scan must read the live instance")
	require.NotContains(t, joined, "t-0", "the scan must not read terminal instance records")
}

func TestFetchRecoverableDropsDanglingIndexKeys(t *testing.T) {
	t.Parallel()
	s, kv := newStoreWithKV(t)
	ctx := t.Context()

	// A crash between the index write and the instance write: no instance.
	_, err := kv.Put(ctx, "=rec.ghost", []byte{'1'})
	require.NoError(t, err)
	// A terminal instance whose index key outlived the terminal write.
	require.NoError(t, s.Create(ctx, &saga.Instance{ID: "done", Definition: "d", Status: saga.StatusCompleted}))
	_, err = kv.Put(ctx, "=rec.done", []byte{'1'})
	require.NoError(t, err)

	rec, err := s.FetchRecoverable(ctx, "d", time.Now(), 10)
	require.NoError(t, err)
	require.Empty(t, rec)
	require.Equal(t, []string{"ghost"}, indexKeys(t, kv), "a key of a terminal instance goes; a fresh one without an instance is kept for the grace period")
}

func TestFetchRecoverableReportsCorruptRecordForQuarantine(t *testing.T) {
	t.Parallel()
	s, kv := newStoreWithKV(t)
	ctx := t.Context()

	_, err := kv.Put(ctx, "a-corrupt", []byte("{not json"))
	require.NoError(t, err)
	_, err = kv.Put(ctx, "=rec.a-corrupt", []byte{'1'})
	require.NoError(t, err)
	require.NoError(t, s.Create(ctx, &saga.Instance{ID: "b-valid", Definition: "d", Status: saga.StatusCompensating}))

	rec, err := s.FetchRecoverable(ctx, "d", time.Now(), 1)
	require.NoError(t, err)
	require.Len(t, rec, 1)
	stub := rec[0]
	require.True(t, stub.Corrupt)
	require.Equal(t, "a-corrupt", stub.ID)

	// The orchestrator's quarantine: a revision-checked Update to Failed.
	stale := &saga.Instance{ID: stub.ID, Version: stub.Version - 1, Status: saga.StatusFailed}
	require.ErrorIs(t, s.Update(ctx, stale), saga.ErrVersionConflict)
	require.NoError(t, s.Update(ctx, &saga.Instance{ID: stub.ID, Version: stub.Version, Status: saga.StatusFailed, Definition: "d"}))

	rec, err = s.FetchRecoverable(ctx, "d", time.Now(), 10)
	require.NoError(t, err)
	require.Len(t, rec, 1)
	require.Equal(t, "b-valid", rec[0].ID, "the quarantined record is gone from the scan")
	require.Equal(t, []string{"b-valid"}, indexKeys(t, kv))
}

// The scan stops early on a bounded batch; the listing's producer must not stay
// blocked on a full channel, one goroutine per call. Serial on purpose: it
// counts the process's goroutines.
func TestBoundedScanReleasesTheKeyLister(t *testing.T) {
	s, _ := newStoreWithKV(t)
	ctx := t.Context()
	for i := range 300 { // More than the lister's channel capacity.
		require.NoError(t, s.Create(ctx, &saga.Instance{ID: fmt.Sprintf("c-%03d", i), Definition: "d", Status: saga.StatusCompensating}))
	}
	before := runtime.NumGoroutine()
	for range 20 {
		rec, err := s.FetchRecoverable(ctx, "d", time.Now(), 1)
		require.NoError(t, err)
		require.Len(t, rec, 1)
	}
	testhelpers.WaitFor(t, 5*time.Second, func() bool { return runtime.NumGoroutine() <= before+4 },
		"goroutines of bounded scans must be released")
}

func TestIDCannotCollideWithIndexNamespace(t *testing.T) {
	t.Parallel()
	s, _ := newStoreWithKV(t)
	ctx := t.Context()

	require.NoError(t, s.Create(ctx, &saga.Instance{ID: "rec.x", Definition: "d", Status: saga.StatusCompleted}))
	require.NoError(t, s.Create(ctx, &saga.Instance{ID: "x", Definition: "d", Status: saga.StatusRunning}))

	n, err := s.DeleteTerminal(ctx, "d", time.Now().Add(time.Hour), 0)
	require.NoError(t, err)
	require.Equal(t, 1, n)
	_, err = s.Get(ctx, "x")
	require.NoError(t, err)
}
