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
		if !h.Enabled(context.Background(), slog.LevelInfo) {
			t.Error("expected Enabled=true when one child is enabled")
		}
	})

	t.Run("false if none enabled", func(t *testing.T) {
		h := NewHandler(newSpy(false), newSpy(false))
		if h.Enabled(context.Background(), slog.LevelInfo) {
			t.Error("expected Enabled=false when no child is enabled")
		}
	})
}

func TestHandle(t *testing.T) {
	s1 := newSpy(true)
	s2 := newSpy(true)
	s3 := newSpy(false)
	h := NewHandler(s1, s2, s3)

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(s1.records) != 1 {
		t.Errorf("s1 got %d records, want 1", len(s1.records))
	}
	if len(s2.records) != 1 {
		t.Errorf("s2 got %d records, want 1", len(s2.records))
	}
	if len(s3.records) != 0 {
		t.Errorf("s3 (disabled) got %d records, want 0", len(s3.records))
	}
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
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errA) {
		t.Errorf("error should wrap errA")
	}
	if !errors.Is(err, errB) {
		t.Errorf("error should wrap errB")
	}
}

func TestWithAttrs(t *testing.T) {
	s1 := newSpy(true)
	s2 := newSpy(true)
	original := NewHandler(s1, s2)

	attrs := []slog.Attr{slog.String("key", "val")}
	derived := original.WithAttrs(attrs)

	// Original should be unchanged.
	for _, child := range original.Handlers() {
		if sp, ok := child.(*spy); ok && len(sp.attrs) > 0 {
			t.Error("original handler children should not have attrs")
		}
	}

	// Derived children should have attrs.
	dh := derived.(*Handler)
	for _, child := range dh.Handlers() {
		sp := child.(*spy)
		if len(sp.attrs) != 1 || sp.attrs[0].Key != "key" {
			t.Errorf("derived child attrs = %v, want [{key val}]", sp.attrs)
		}
	}
}

func TestWithGroup(t *testing.T) {
	s1 := newSpy(true)
	s2 := newSpy(true)
	original := NewHandler(s1, s2)

	derived := original.WithGroup("grp")

	// Original should be unchanged.
	for _, child := range original.Handlers() {
		if sp, ok := child.(*spy); ok && len(sp.groups) > 0 {
			t.Error("original handler children should not have groups")
		}
	}

	// Derived children should have the group.
	dh := derived.(*Handler)
	for _, child := range dh.Handlers() {
		sp := child.(*spy)
		if len(sp.groups) != 1 || sp.groups[0] != "grp" {
			t.Errorf("derived child groups = %v, want [grp]", sp.groups)
		}
	}
}

func TestWithGroup_Empty(t *testing.T) {
	h := NewHandler(newSpy(true))
	if h.WithGroup("") != h {
		t.Error("WithGroup(\"\") should return same handler")
	}
}

func TestHandlers(t *testing.T) {
	s1 := newSpy(true)
	s2 := newSpy(false)
	h := NewHandler(s1, s2)

	children := h.Handlers()
	if len(children) != 2 {
		t.Fatalf("len(Handlers()) = %d, want 2", len(children))
	}
}

func TestNewHandler_Empty(t *testing.T) {
	h := NewHandler()
	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("empty handler should not be enabled")
	}
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Errorf("empty handler Handle should return nil, got %v", err)
	}
	if len(h.Handlers()) != 0 {
		t.Errorf("empty handler should have 0 children")
	}
}

func TestNewHandler_DefensiveCopy(t *testing.T) {
	spies := []slog.Handler{newSpy(true), newSpy(true)}
	h := NewHandler(spies...)

	// Mutate original slice.
	spies[0] = newSpy(false)

	// Handler should still have the original spy.
	if !h.Handlers()[0].Enabled(context.Background(), slog.LevelInfo) {
		t.Error("handler should retain original child after external slice mutation")
	}
}

// --- Concurrent handler tests ---

func TestHandle_Concurrent(t *testing.T) {
	s1 := newSpy(true)
	s2 := newSpy(true)
	s3 := newSpy(false)
	h := NewConcurrentHandler(s1, s2, s3)

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s1.mu.Lock()
	defer s1.mu.Unlock()
	s2.mu.Lock()
	defer s2.mu.Unlock()
	s3.mu.Lock()
	defer s3.mu.Unlock()

	if len(s1.records) != 1 {
		t.Errorf("s1 got %d records, want 1", len(s1.records))
	}
	if len(s2.records) != 1 {
		t.Errorf("s2 got %d records, want 1", len(s2.records))
	}
	if len(s3.records) != 0 {
		t.Errorf("s3 (disabled) got %d records, want 0", len(s3.records))
	}
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
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errA) {
		t.Errorf("error should wrap errA")
	}
	if !errors.Is(err, errB) {
		t.Errorf("error should wrap errB")
	}
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
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)

	// With concurrent dispatch, the total time should be ~1s (the slow
	// handler's delay), not 2s. We allow some margin.
	if elapsed > 2*time.Second {
		t.Errorf("concurrent dispatch took %v, expected ~1s", elapsed)
	}

	fast.mu.Lock()
	defer fast.mu.Unlock()
	if len(fast.records) != 1 {
		t.Errorf("fast handler got %d records, want 1", len(fast.records))
	}
}

func TestWithAttrs_Concurrent(t *testing.T) {
	s1 := newSpy(true)
	s2 := newSpy(true)
	original := NewConcurrentHandler(s1, s2)

	attrs := []slog.Attr{slog.String("key", "val")}
	derived := original.WithAttrs(attrs)

	dh := derived.(*Handler)
	if !dh.concurrent {
		t.Error("WithAttrs should propagate concurrent flag")
	}
	for _, child := range dh.Handlers() {
		sp := child.(*spy)
		if len(sp.attrs) != 1 || sp.attrs[0].Key != "key" {
			t.Errorf("derived child attrs = %v, want [{key val}]", sp.attrs)
		}
	}
}

func TestWithGroup_Concurrent(t *testing.T) {
	s1 := newSpy(true)
	s2 := newSpy(true)
	original := NewConcurrentHandler(s1, s2)

	derived := original.WithGroup("grp")

	dh := derived.(*Handler)
	if !dh.concurrent {
		t.Error("WithGroup should propagate concurrent flag")
	}
	for _, child := range dh.Handlers() {
		sp := child.(*spy)
		if len(sp.groups) != 1 || sp.groups[0] != "grp" {
			t.Errorf("derived child groups = %v, want [grp]", sp.groups)
		}
	}
}
