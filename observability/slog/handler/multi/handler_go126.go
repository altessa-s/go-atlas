// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build go1.26

package multi

import (
	"context"
	"log/slog"
	"slices"
)

// Handler fans out log records to multiple [slog.Handler] implementations.
// On Go 1.26+ this delegates to [slog.MultiHandler] from the standard library.
type Handler struct {
	inner      *slog.MultiHandler
	handlers   []slog.Handler
	concurrent bool
}

// NewHandler creates a Handler that dispatches to all provided handlers.
// The input slice is defensively copied.
func NewHandler(handlers ...slog.Handler) *Handler {
	cloned := slices.Clone(handlers)
	return &Handler{
		inner:    slog.NewMultiHandler(cloned...),
		handlers: cloned,
	}
}

// NewConcurrentHandler creates a Handler that dispatches to all provided
// handlers concurrently using [concurrency.Process]. This is useful when one
// or more children may block (e.g. a network logger) and you do not want to
// delay the remaining handlers.
func NewConcurrentHandler(handlers ...slog.Handler) *Handler {
	cloned := slices.Clone(handlers)
	return &Handler{
		inner:      slog.NewMultiHandler(cloned...),
		handlers:   cloned,
		concurrent: true,
	}
}

// Enabled reports whether any child handler is enabled for the given level.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle dispatches the record to all enabled child handlers.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if h.concurrent {
		return handleConcurrent(ctx, r, h.handlers)
	}
	return h.inner.Handle(ctx, r)
}

// WithAttrs returns a new Handler with attrs applied to every child.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	children := make([]slog.Handler, len(h.handlers))
	for i, child := range h.handlers {
		children[i] = child.WithAttrs(attrs)
	}
	return &Handler{
		inner:      slog.NewMultiHandler(children...),
		handlers:   children,
		concurrent: h.concurrent,
	}
}

// WithGroup returns a new Handler with the group applied to every child.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	children := make([]slog.Handler, len(h.handlers))
	for i, child := range h.handlers {
		children[i] = child.WithGroup(name)
	}
	return &Handler{
		inner:      slog.NewMultiHandler(children...),
		handlers:   children,
		concurrent: h.concurrent,
	}
}

// Handlers returns a copy of the child handlers. Used by shutdown traversal.
func (h *Handler) Handlers() []slog.Handler {
	return slices.Clone(h.handlers)
}
