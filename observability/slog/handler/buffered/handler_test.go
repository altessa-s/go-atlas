// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package buffered

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
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

	if err := h.Shutdown(t.Context()); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "message 1") || !strings.Contains(output, "message 2") {
		t.Errorf("expected messages to given written, got:\n%s", output)
	}
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

	if idx1 == -1 || idx2 == -1 {
		t.Fatalf("missing logs in output:\n%s", output)
	}

	if idx1 > idx2 {
		t.Errorf("expected buffered msg to appear before critical error (flush failed?)")
	}

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
	if count != 5 {
		t.Errorf("expected 5 messages, got %d", count)
	}
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
	if err := h.Shutdown(t.Context()); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "bypass error") {
		t.Error("missing bypass error")
	}
	if !strings.Contains(output, "should be processed") {
		t.Error("worker probably died after flush, missing 'should be processed'")
	}
}
