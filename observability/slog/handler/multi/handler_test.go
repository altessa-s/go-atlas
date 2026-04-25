// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package multi

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// spy is a test handler that records calls.
type spy struct {
	mu      sync.Mutex
	enabled bool
	records []slog.Record
	attrs   []slog.Attr
	groups  []string
	err     error
}

func newSpy(enabled bool) *spy { return &spy{enabled: enabled} }

func (s *spy) Enabled(context.Context, slog.Level) bool { return s.enabled }

func (s *spy) Handle(_ context.Context, r slog.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, r)
	return s.err
}

func (s *spy) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &spy{enabled: s.enabled, attrs: attrs, err: s.err}
}

func (s *spy) WithGroup(name string) slog.Handler {
	return &spy{enabled: s.enabled, groups: []string{name}, err: s.err}
}

func TestEnabled(t *testing.T) {
	t.Run("true if any child enabled", func(t *testing.T) {
		h := NewHandler(newSpy(false), newSpy(true), newSpy(false))
		require.True(t, h.Enabled(context.Background(), slog.LevelInfo))
	})

	t.Run("false if none enabled", func(t *testing.T) {
		h := NewHandler(newSpy(false), newSpy(false))
		require.False(t, h.Enabled(context.Background(), slog.LevelInfo))
	})
}

func TestHandle(t *testing.T) {
	s1 := newSpy(true)
	s2 := newSpy(true)
	s3 := newSpy(false)
	h := NewHandler(s1, s2, s3)

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)
	require.NoError(t, h.Handle(context.Background(), r))

	require.Len(t, s1.records, 1)
	require.Len(t, s2.records, 1)
	require.Empty(t, s3.records)
}

func TestHandle_ErrorCollection(t *testing.T) {
	errA := errors.New("handler A failed")
	errB := errors.New("handler B failed")

	s1 := newSpy(true)
	s1.err = errA
	s2 := newSpy(true)
	s2.err = errB
	h := NewHandler(s1, s2)

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	err := h.Handle(context.Background(), r)
	require.Error(t, err)
	require.ErrorIs(t, err, errA)
	require.ErrorIs(t, err, errB)
}

func TestWithAttrs(t *testing.T) {
	s1 := newSpy(true)
	s2 := newSpy(true)
	original := NewHandler(s1, s2)

	attrs := []slog.Attr{slog.String("key", "val")}
	derived := original.WithAttrs(attrs)

	// Original should be unchanged.
	for _, child := range original.Handlers() {
		if sp, ok := child.(*spy); ok {
			require.Empty(t, sp.attrs, "original handler children should not have attrs")
		}
	}

	// Derived children should have attrs.
	dh := derived.(*Handler)
	for _, child := range dh.Handlers() {
		sp := child.(*spy)
		require.Len(t, sp.attrs, 1)
		require.Equal(t, "key", sp.attrs[0].Key)
	}
}

func TestWithGroup(t *testing.T) {
	s1 := newSpy(true)
	s2 := newSpy(true)
	original := NewHandler(s1, s2)

	derived := original.WithGroup("grp")

	// Original should be unchanged.
	for _, child := range original.Handlers() {
		if sp, ok := child.(*spy); ok {
			require.Empty(t, sp.groups, "original handler children should not have groups")
		}
	}

	// Derived children should have the group.
	dh := derived.(*Handler)
	for _, child := range dh.Handlers() {
		sp := child.(*spy)
		require.Len(t, sp.groups, 1)
		require.Equal(t, "grp", sp.groups[0])
	}
}

func TestWithGroup_Empty(t *testing.T) {
	h := NewHandler(newSpy(true))
	require.Equal(t, h, h.WithGroup(""))
}

func TestHandlers(t *testing.T) {
	s1 := newSpy(true)
	s2 := newSpy(false)
	h := NewHandler(s1, s2)

	children := h.Handlers()
	require.Len(t, children, 2)
}

func TestNewHandler_Empty(t *testing.T) {
	h := NewHandler()
	require.False(t, h.Enabled(context.Background(), slog.LevelInfo))
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	require.NoError(t, h.Handle(context.Background(), r))
	require.Empty(t, h.Handlers())
}

func TestNewHandler_DefensiveCopy(t *testing.T) {
	spies := []slog.Handler{newSpy(true), newSpy(true)}
	h := NewHandler(spies...)

	// Mutate original slice.
	spies[0] = newSpy(false)

	// Handler should still have the original spy.
	require.True(t, h.Handlers()[0].Enabled(context.Background(), slog.LevelInfo), "handler should retain original child after external slice mutation")
}

// --- Concurrent handler tests ---

func TestHandle_Concurrent(t *testing.T) {
	s1 := newSpy(true)
	s2 := newSpy(true)
	s3 := newSpy(false)
	h := NewConcurrentHandler(s1, s2, s3)

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)
	require.NoError(t, h.Handle(context.Background(), r))

	s1.mu.Lock()
	defer s1.mu.Unlock()
	s2.mu.Lock()
	defer s2.mu.Unlock()
	s3.mu.Lock()
	defer s3.mu.Unlock()

	require.Len(t, s1.records, 1)
	require.Len(t, s2.records, 1)
	require.Empty(t, s3.records)
}

func TestHandle_Concurrent_ErrorCollection(t *testing.T) {
	errA := errors.New("handler A failed")
	errB := errors.New("handler B failed")

	s1 := newSpy(true)
	s1.err = errA
	s2 := newSpy(true)
	s2.err = errB
	h := NewConcurrentHandler(s1, s2)

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	err := h.Handle(context.Background(), r)
	require.Error(t, err)
	require.ErrorIs(t, err, errA)
	require.ErrorIs(t, err, errB)
}

// slowSpy is a handler that sleeps before recording.
type slowSpy struct {
	spy
	delay time.Duration
}

func (s *slowSpy) Handle(ctx context.Context, r slog.Record) error {
	time.Sleep(s.delay)
	return s.spy.Handle(ctx, r)
}

func (s *slowSpy) Enabled(ctx context.Context, level slog.Level) bool {
	return s.spy.Enabled(ctx, level)
}

func (s *slowSpy) WithAttrs(attrs []slog.Attr) slog.Handler { return s }
func (s *slowSpy) WithGroup(name string) slog.Handler       { return s }

func TestHandle_Concurrent_SlowHandler(t *testing.T) {
	slow := &slowSpy{spy: spy{enabled: true}, delay: time.Second}
	fast := newSpy(true)
	h := NewConcurrentHandler(slow, fast)

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)

	start := time.Now()
	require.NoError(t, h.Handle(context.Background(), r))
	elapsed := time.Since(start)

	// With concurrent dispatch, the total time should be ~1s (the slow
	// handler's delay), not 2s. We allow some margin.
	require.Less(t, elapsed, 2*time.Second, "concurrent dispatch took %v, expected ~1s", elapsed)

	fast.mu.Lock()
	defer fast.mu.Unlock()
	require.Len(t, fast.records, 1)
}

func TestWithAttrs_Concurrent(t *testing.T) {
	s1 := newSpy(true)
	s2 := newSpy(true)
	original := NewConcurrentHandler(s1, s2)

	attrs := []slog.Attr{slog.String("key", "val")}
	derived := original.WithAttrs(attrs)

	dh := derived.(*Handler)
	require.True(t, dh.concurrent, "WithAttrs should propagate concurrent flag")
	for _, child := range dh.Handlers() {
		sp := child.(*spy)
		require.Len(t, sp.attrs, 1)
		require.Equal(t, "key", sp.attrs[0].Key)
	}
}

func TestWithGroup_Concurrent(t *testing.T) {
	s1 := newSpy(true)
	s2 := newSpy(true)
	original := NewConcurrentHandler(s1, s2)

	derived := original.WithGroup("grp")

	dh := derived.(*Handler)
	require.True(t, dh.concurrent, "WithGroup should propagate concurrent flag")
	for _, child := range dh.Handlers() {
		sp := child.(*spy)
		require.Len(t, sp.groups, 1)
		require.Equal(t, "grp", sp.groups[0])
	}
}
