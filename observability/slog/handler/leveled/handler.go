// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leveled

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/slog/handler/internal/base"
)

// Handler wraps a [slog.Handler] and filters log records based on per-subsystem levels.
//
// When the subsystem is captured via [Handler.WithAttrs] (i.e. the logger was created
// with slogx.Module), [Handler.Enabled] performs a single integer comparison.
// Otherwise, [Handler.Handle] scans record attributes to find the subsystem key.
type Handler struct {
	base.Base
	opts           *options
	subsystem      string     // captured via WithAttrs
	subsystemLevel slog.Level // resolved level for captured subsystem
	hasSubsys      bool       // true when subsystem is known
}

// NewHandler creates a leveled handler wrapping next.
func NewHandler(next slog.Handler, opts ...Option) *Handler {
	o := newOptions(opts...)
	return &Handler{
		Base: base.NewBase(next),
		opts: o,
	}
}

// Enabled reports whether a record at the given level should be logged.
//
// When the subsystem is known (captured via WithAttrs), it checks against the
// subsystem-specific level. Otherwise, it delegates to the base handler which
// uses the minimum level across all subsystems.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	if h.hasSubsys {
		return level >= h.subsystemLevel
	}
	return h.Base.Enabled(ctx, level)
}

// Handle filters the record based on subsystem level when the subsystem is not
// already known. If the subsystem was captured via WithAttrs, this is a pass-through.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if h.hasSubsys {
		return h.Inner().Handle(ctx, r)
	}

	subsystem := h.findSubsystem(r)
	if r.Level < h.levelForSubsystem(subsystem) {
		return nil
	}

	return h.Inner().Handle(ctx, r)
}

// WithAttrs returns a new Handler with the given attributes.
// If any attribute matches the subsystem key, the subsystem is captured for fast-path filtering.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	subsystem, subsystemLevel, hasSubsys := h.subsystem, h.subsystemLevel, h.hasSubsys

	if !hasSubsys {
		for _, a := range attrs {
			if a.Key == h.opts.subsystemKey && a.Value.Kind() == slog.KindString {
				subsystem = a.Value.String()
				subsystemLevel = h.levelForSubsystem(subsystem)
				hasSubsys = true
				break
			}
		}
	}

	return &Handler{
		Base:           h.WithAttrsBase(attrs),
		opts:           h.opts,
		subsystem:      subsystem,
		subsystemLevel: subsystemLevel,
		hasSubsys:      hasSubsys,
	}
}

// WithGroup returns a new Handler with the given group name.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return &Handler{
		Base:           h.WithGroupBase(name),
		opts:           h.opts,
		subsystem:      h.subsystem,
		subsystemLevel: h.subsystemLevel,
		hasSubsys:      h.hasSubsys,
	}
}

// levelForSubsystem returns the configured level for the given subsystem,
// falling back to the default level.
func (h *Handler) levelForSubsystem(name string) slog.Level {
	if lvl, ok := h.opts.subsystemLevels[name]; ok {
		return lvl
	}
	return h.opts.defaultLevel
}

// findSubsystem scans record attributes for the subsystem key.
// Returns the first matching string value, or empty string if not found.
func (h *Handler) findSubsystem(r slog.Record) string {
	var subsystem string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == h.opts.subsystemKey && a.Value.Kind() == slog.KindString {
			subsystem = a.Value.String()
			return false
		}
		return true
	})
	return subsystem
}
