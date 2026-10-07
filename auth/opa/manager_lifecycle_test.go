// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa"
)

// gatedSource serves a fixed bundle. While gated, each Fetch signals entered
// and then blocks until release is closed, ignoring its context like a slow
// source would. It records Close calls and any Fetch that overlaps or follows
// Close.
type gatedSource struct {
	gated   atomic.Bool
	entered chan struct{}
	release chan struct{}

	inFetch    atomic.Int32
	closeCalls atomic.Int32
	violations atomic.Int32 // Fetch after Close, or Close during Fetch
}

func newGatedSource() *gatedSource {
	return &gatedSource{entered: make(chan struct{}, 64), release: make(chan struct{})}
}

func (s *gatedSource) Name() string { return "gated" }

func (s *gatedSource) Fetch(context.Context) (*opa.PolicyBundle, error) {
	s.inFetch.Add(1)
	defer s.inFetch.Add(-1)
	if s.closeCalls.Load() > 0 {
		s.violations.Add(1)
	}
	if s.gated.Load() {
		s.entered <- struct{}{}
		<-s.release
	}
	return &opa.PolicyBundle{
		Revision: "r1",
		Modules:  map[string][]byte{"p.rego": []byte("package t\nimport rego.v1\nallow if true\n")},
	}, nil
}

func (s *gatedSource) Close() error {
	if s.inFetch.Load() > 0 {
		s.violations.Add(1)
	}
	s.closeCalls.Add(1)
	return nil
}

func newGatedManager(t *testing.T, src *gatedSource) *opa.Manager {
	t.Helper()
	m, err := opa.NewManager(t.Context(), src, "data.t.allow", opa.WithPollInterval(time.Millisecond))
	require.NoError(t, err)
	return m
}

// releaseLater unblocks the gated Fetch shortly after the caller starts
// waiting. The delay only keeps the test live; the assertions hold for any
// interleaving because they inspect state at the moment the waiter returns.
func releaseLater(src *gatedSource) {
	go func() {
		time.Sleep(20 * time.Millisecond)
		src.gated.Store(false)
		close(src.release)
	}()
}

// TestManager_StopWatchingJoinsPollLoop: StopWatching must not return while
// the poll loop is still inside Fetch.
func TestManager_StopWatchingJoinsPollLoop(t *testing.T) {
	t.Parallel()
	src := newGatedSource()
	m := newGatedManager(t, src)
	t.Cleanup(func() { _ = m.Close() })

	src.gated.Store(true)
	require.NoError(t, m.StartWatching(t.Context()))
	<-src.entered // the loop is blocked inside Fetch

	releaseLater(src)
	m.StopWatching()
	require.Zero(t, src.inFetch.Load(), "StopWatching returned while the poll loop was in Fetch")
}

// TestManager_ConcurrentCloseWaitsForTeardown: no Close caller returns before
// the teardown finished, and the source is closed exactly once, never while
// Fetch is running.
func TestManager_ConcurrentCloseWaitsForTeardown(t *testing.T) {
	t.Parallel()
	src := newGatedSource()
	m := newGatedManager(t, src)

	src.gated.Store(true)
	require.NoError(t, m.StartWatching(t.Context()))
	<-src.entered

	releaseLater(src)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			_ = m.Close()
			if src.inFetch.Load() != 0 || src.closeCalls.Load() != 1 {
				t.Error("Close returned before the poll loop exited and the source was closed")
			}
		})
	}
	wg.Wait()
	require.Equal(t, int32(1), src.closeCalls.Load())
	require.Zero(t, src.violations.Load(), "source closed while Fetch was running")
}

// TestManager_RestartDoesNotLeakPollLoops: a stop/start/Close storm leaves no
// loop running after Close and never fetches after the source is closed.
func TestManager_RestartDoesNotLeakPollLoops(t *testing.T) {
	t.Parallel()
	src := newGatedSource()
	m := newGatedManager(t, src)

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 25 {
				_ = m.StartWatching(t.Context())
				m.StopWatching()
			}
		})
	}
	wg.Go(func() { _ = m.StartWatching(t.Context()) })
	wg.Wait()

	require.NoError(t, m.Close())
	require.ErrorIs(t, m.StartWatching(t.Context()), opa.ErrManagerClosed)
	require.Zero(t, src.inFetch.Load(), "a poll loop is still fetching after Close")
	require.Zero(t, src.violations.Load())
	require.False(t, m.IsWatching())
}
