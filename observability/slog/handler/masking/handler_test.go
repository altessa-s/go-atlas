// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

// captureStore is a shared store that survives handler cloning via WithAttrs/WithGroup.
type captureStore struct {
	attrs map[string]slog.Value
}

type captureHandler struct {
	store *captureStore
}

func newCaptureHandler() (*captureHandler, *captureStore) {
	s := &captureStore{}
	return &captureHandler{store: s}, s
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.store.attrs = make(map[string]slog.Value)
	r.Attrs(func(a slog.Attr) bool {
		h.store.attrs[a.Key] = a.Value
		return true
	})
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler {
	return &captureHandler{store: h.store}
}

func (h *captureHandler) WithGroup(string) slog.Handler {
	return &captureHandler{store: h.store}
}

func TestHandler_WithAttrs_PreservesPatterns(t *testing.T) {
	inner, store := newCaptureHandler()
	h := NewHandler(inner,
		WithPattern(`(?i).*secret.*`, FullMask()),
	)

	h2 := h.WithAttrs([]slog.Attr{slog.String("extra", "val")})

	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	rec.AddAttrs(slog.String("my_secret_key", "sensitive"))

	if err := h2.Handle(t.Context(), rec); err != nil {
		t.Fatalf("Handle err=%v", err)
	}

	got := store.attrs["my_secret_key"].String()
	if got != "********" {
		t.Fatalf("WithAttrs lost patterns: got %q, want %q", got, "********")
	}
}

func TestHandler_WithGroup_PreservesPatterns(t *testing.T) {
	inner, store := newCaptureHandler()
	h := NewHandler(inner,
		WithPattern(`(?i).*secret.*`, FullMask()),
	)

	h2 := h.WithGroup("group1")

	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	rec.AddAttrs(slog.String("my_secret_key", "sensitive"))

	if err := h2.Handle(t.Context(), rec); err != nil {
		t.Fatalf("Handle err=%v", err)
	}

	got := store.attrs["my_secret_key"].String()
	if got != "********" {
		t.Fatalf("WithGroup lost patterns: got %q, want %q", got, "********")
	}
}

func TestHandler_MasksByFieldPattern(t *testing.T) {
	inner, store := newCaptureHandler()
	h := NewHandler(inner,
		WithMaskNestedFields(),
		WithCaseSensitive(),
		WithPattern(`(?i).*password.*`, FullMask()),
	).(*Handler)

	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "msg", 0)
	rec.AddAttrs(slog.String("Password", "secret123"))

	if err := h.Handle(t.Context(), rec); err != nil {
		t.Fatalf("Handle err=%v", err)
	}

	got := store.attrs["Password"].String()
	if got != "********" {
		t.Fatalf("masked=%q, want %q", got, "********")
	}
}
