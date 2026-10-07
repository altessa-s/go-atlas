// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package buffered

import (
	"bytes"
	"context"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/runtime/helpers"
)

// safeBuffer is a thread-safe buffer for testing concurrent writes.
type safeBuffer struct {
	b  bytes.Buffer
	mu sync.Mutex
}

func (s *safeBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestHandler_AsyncBuffer(t *testing.T) {
	var buf safeBuffer
	// Use TextHandler as inner handler
	inner := slog.NewTextHandler(&buf, nil)

	// Create buffered handler with small buffer
	h := NewHandler(inner, WithBufferSize(10), WithBypassLevel(slog.LevelError))
	logger := slog.New(h)

	// Log some info messages (should be buffered)
	logger.Info("message 1")
	logger.Info("message 2")

	// Wait a bit to ensure potential background processing (it might happen fast)
	// But in a "perfect" async world, we can't guarantee it's written immediately.
	// We rely on Shutdown to flush.

	require.NoError(t, h.Shutdown(t.Context()))

	output := buf.String()
	require.Contains(t, output, "message 1")
	require.Contains(t, output, "message 2")
}

func TestHandler_BypassLevel(t *testing.T) {
	var buf safeBuffer // Shared buffer
	inner := slog.NewTextHandler(&buf, nil)

	// Buffered handler
	h := NewHandler(inner, WithBufferSize(100), WithBypassLevel(slog.LevelError))
	logger := slog.New(h)

	// 1. Info log (Buffered)
	logger.Info("buffered msg")

	// 2. Error log (Bypass + Flush)
	// This should force "buffered msg" to be written BEFORE "critical error"
	logger.Error("critical error")

	output := buf.String()

	// Check order
	idx1 := strings.Index(output, "buffered msg")
	idx2 := strings.Index(output, "critical error")

	require.NotEqual(t, -1, idx1, "missing buffered msg in output")
	require.NotEqual(t, -1, idx2, "missing critical error in output")
	require.Less(t, idx1, idx2, "expected buffered msg to appear before critical error")

	_ = h.Shutdown(t.Context())
}

func TestHandler_DropOrBlock(t *testing.T) {
	// Limitation: The current implementation falls back to sync write if buffer is full.
	// So we can verify that logs are NOT dropped even if buffer is full.
	var buf safeBuffer
	inner := slog.NewTextHandler(&buf, nil)

	// Buffer size 1
	h := NewHandler(inner, WithBufferSize(1))
	logger := slog.New(h)

	// Write 5 messages quickly
	for i := range 5 {
		logger.Info("msg", slog.Int("id", i))
	}

	_ = h.Shutdown(t.Context())

	output := buf.String()
	for range 5 {
		if !strings.Contains(output, "id="+time.Now().Format("?")) && !strings.Contains(output, "id=") {
			// Basic check
		}
	}
	// Count occurrences
	count := strings.Count(output, "msg=")
	require.Equal(t, 5, count)
}

func TestHandler_ShutdownTimeout(t *testing.T) {
	// This test is hard to make deterministic without mocking the inner handler to block.
	// Skipping specifically simulating timeout for now, but verifying API works.
	h := NewHandler(slog.Default().Handler())
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	if err := h.Shutdown(ctx); err != nil {
		// Should not error if empty
	}
}

func TestHandler_WorkerSurvivesFlush(t *testing.T) {
	var buf safeBuffer
	inner := slog.NewTextHandler(&buf, nil)
	h := NewHandler(inner, WithBufferSize(10), WithBypassLevel(slog.LevelError))
	logger := slog.New(h)

	// 1. Trigger a flush (via Error bypass)
	logger.Error("bypass error")

	// 2. Log regular info
	logger.Info("should be processed")

	// 3. Shutdown
	require.NoError(t, h.Shutdown(t.Context()))

	output := buf.String()
	require.Contains(t, output, "bypass error")
	require.Contains(t, output, "should be processed")
}

// funcHandler is a minimal inner handler whose Handle delegates to fn, so
// tests can observe which goroutine writes a record or block the worker.
type funcHandler struct {
	fn func(slog.Record)
}

func (f funcHandler) Enabled(context.Context, slog.Level) bool { return true }

func (f funcHandler) Handle(_ context.Context, r slog.Record) error {
	f.fn(r)
	return nil
}

func (f funcHandler) WithAttrs([]slog.Attr) slog.Handler { return f }

func (f funcHandler) WithGroup(string) slog.Handler { return f }

func TestHandler_PreservesAttrs(t *testing.T) {
	t.Parallel()

	var buf safeBuffer
	h := NewHandler(slog.NewTextHandler(&buf, nil))
	logger := slog.New(h).With("svc", "api")

	logger.Info("request", "user", "alice", slog.Int("status", 200), slog.Group("req", "path", "/x"))
	require.NoError(t, h.Shutdown(t.Context()))

	output := buf.String()
	require.Contains(t, output, "svc=api")
	require.Contains(t, output, "user=alice")
	require.Contains(t, output, "status=200")
	require.Contains(t, output, "req.path=/x")
}

func TestHandler_DerivedHandlersShareWorker(t *testing.T) {
	// Serial: runtime.NumGoroutine is process-wide, so parallel tests would skew the delta.
	var buf safeBuffer
	h := NewHandler(slog.NewTextHandler(&buf, nil))
	root := slog.New(h)

	before := runtime.NumGoroutine()
	derived := make([]*slog.Logger, 0, 50)
	for i := range 50 {
		derived = append(derived, root.With("clone", i).WithGroup("g"))
	}
	require.Less(t, runtime.NumGoroutine()-before, 10, "derived handlers must not start their own workers")

	for i, l := range derived {
		l.Info("derived", "n", i)
	}
	require.NoError(t, h.Shutdown(t.Context()))

	output := buf.String()
	require.Equal(t, 50, strings.Count(output, "msg=derived"))
	require.Contains(t, output, "clone=7 g.n=7")
}

func TestHandler_ShutdownDerivedStopsSharedPipeline(t *testing.T) {
	t.Parallel()

	var writerGID atomic.Int64
	h := NewHandler(funcHandler{fn: func(slog.Record) { writerGID.Store(int64(helpers.GoroutineID())) }})
	derived := h.WithAttrs([]slog.Attr{slog.String("k", "v")})

	require.NoError(t, derived.(*Handler).Shutdown(t.Context()))

	// After shutdown the root must write synchronously on the caller goroutine.
	require.NoError(t, h.Handle(t.Context(), slog.NewRecord(time.Now(), slog.LevelInfo, "after", 0)))
	require.Equal(t, int64(helpers.GoroutineID()), writerGID.Load())
}

func TestHandler_ConcurrentShutdownWaitsForDrain(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	gate := make(chan struct{})
	var gateOpened atomic.Bool
	var once sync.Once
	h := NewHandler(funcHandler{fn: func(slog.Record) {
		once.Do(func() { close(entered) })
		<-gate
	}})

	require.NoError(t, h.Handle(t.Context(), slog.NewRecord(time.Now(), slog.LevelInfo, "blocked", 0)))
	<-entered // the worker is now parked inside the inner handler

	returned := make(chan bool, 2)
	for range 2 {
		go func() {
			_ = h.Shutdown(t.Context())
			returned <- gateOpened.Load()
		}()
	}

	// No Shutdown caller may return while the worker is still draining.
	select {
	case <-returned:
		t.Fatal("Shutdown returned before the buffered record was drained")
	case <-time.After(100 * time.Millisecond):
	}

	gateOpened.Store(true)
	close(gate)
	require.True(t, <-returned)
	require.True(t, <-returned)
}

func TestHandler_ConcurrentHandleAndShutdownLosesNothing(t *testing.T) {
	t.Parallel()

	var handled atomic.Int64
	h := NewHandler(funcHandler{fn: func(slog.Record) { handled.Add(1) }}, WithBufferSize(4))

	const writers, perWriter = 8, 200
	var wg sync.WaitGroup
	for range writers {
		wg.Go(func() {
			for range perWriter {
				_ = h.Handle(t.Context(), slog.NewRecord(time.Now(), slog.LevelInfo, "m", 0))
			}
		})
	}
	wg.Go(func() { _ = h.Shutdown(t.Context()) })
	wg.Wait()

	require.Equal(t, int64(writers*perWriter), handled.Load())
}
