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

type captureHandler struct {
	attrs map[string]slog.Value
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.attrs = make(map[string]slog.Value)
	r.Attrs(func(a slog.Attr) bool {
		h.attrs[a.Key] = a.Value
		return true
	})
	return nil
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// minimal: apply attrs at construction time by returning a handler that will emit them on Handle
	ch := &captureHandler{attrs: make(map[string]slog.Value)}
	for _, a := range attrs {
		ch.attrs[a.Key] = a.Value
	}
	return ch
}

func (h *captureHandler) WithGroup(string) slog.Handler { return h }

func TestHandler_MasksByFieldPattern(t *testing.T) {
	inner := &captureHandler{}
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

	got := inner.attrs["Password"].String()
	if got != "********" {
		t.Fatalf("masked=%q, want %q", got, "********")
	}
}
