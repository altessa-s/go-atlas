// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prefixed

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/slog/handler/internal/base"
)

// Handler wraps a [slog.Handler] to extract and format prefixes from log records.
// Prefix values are string attributes under the key set with [WithPrefix]; they are
// removed from the output attributes and replaced by one formatted prefix attribute
// per group level, at the level they were attached to.
type Handler struct {
	base.Base
	opts *options
	// prefixes holds the prefix values collected at the current group level
	// that have not been passed to the inner handler yet.
	prefixes []slog.Value
}

// NewHandler creates a prefixed handler wrapping next.
//
// Example:
//
//	h := prefixed.NewHandler(slog.NewTextHandler(os.Stdout, nil))
//	logger := slog.New(h)
func NewHandler(next slog.Handler, opts ...Option) *Handler {
	o := newOptions(opts...)

	// Set default formatter if not provided
	if o.prefixFormatter == nil {
		o.prefixFormatter = DefaultFormatter
	}

	return &Handler{
		Base:     base.NewBase(next),
		opts:     o,
		prefixes: make([]slog.Value, 0, 4),
	}
}

// Enabled reports whether the underlying handler handles records at this level.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.Base.Enabled(ctx, level)
}

// Handle extracts prefixes, formats them, and passes the record to the next handler.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	// Treat h.prefixes as immutable. Only allocate when the record carries
	// prefixes of its own that have to be merged and removed.
	prefixes := h.prefixes
	if h.recordHasPrefix(r) {
		attrs := make([]slog.Attr, 0, r.NumAttrs())
		r.Attrs(func(a slog.Attr) bool {
			attrs = append(attrs, a)
			return true
		})

		var nattrs []slog.Attr
		prefixes, nattrs = h.extractPrefixes(attrs)
		r = slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
		r.AddAttrs(nattrs...)
	} else {
		// r is a shallow copy of the caller's record: clone it so appending
		// the prefix below never writes into storage the caller still shares.
		r = r.Clone()
	}

	if attr, ok := h.prefixAttr(prefixes); ok {
		r.AddAttrs(attr)
	}

	return h.Inner().Handle(ctx, r)
}

// WithAttrs returns a new Handler with the given attributes, extracting prefix attrs.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	p, remaining := h.extractPrefixes(attrs)
	return &Handler{
		Base:     h.WithAttrsBase(remaining),
		opts:     h.opts,
		prefixes: p,
	}
}

// WithGroup returns a new Handler with the given group name.
//
// Prefixes collected at the current level are passed to the inner handler as one
// formatted attribute before the group is opened, so they stay at the level they
// were attached to. The new group starts with no prefixes.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	b := h.Base
	if attr, ok := h.prefixAttr(h.prefixes); ok {
		b = h.WithAttrsBase([]slog.Attr{attr})
	}
	return &Handler{
		Base: b.WithGroupBase(name),
		opts: h.opts,
	}
}

// prefixAttr formats prefixes into the prefix attribute. It reports false when
// there are no prefixes or the formatter returns nil.
func (h *Handler) prefixAttr(prefixes []slog.Value) (slog.Attr, bool) {
	if len(prefixes) == 0 {
		return slog.Attr{}, false
	}
	v := h.opts.prefixFormatter(prefixes, h.opts.prefixesDelimiter)
	if v == nil {
		return slog.Attr{}, false
	}
	return slog.Attr{Key: h.opts.prefix, Value: *v}, true
}

// recordHasPrefix reports whether r carries at least one prefix attribute.
func (h *Handler) recordHasPrefix(r slog.Record) bool {
	found := false
	r.Attrs(func(a slog.Attr) bool {
		found = h.isPrefix(a)
		return !found
	})
	return found
}

// extractPrefixes returns prefix values and remaining non-prefix attributes.
// Without prefix attributes it returns h.prefixes and attrs unchanged.
func (h *Handler) extractPrefixes(attrs []slog.Attr) ([]slog.Value, []slog.Attr) {
	n := 0
	for _, a := range attrs {
		if h.isPrefix(a) {
			n++
		}
	}
	if n == 0 {
		return h.prefixes, attrs
	}

	prefixes := make([]slog.Value, 0, len(h.prefixes)+n)
	prefixes = append(prefixes, h.prefixes...)
	remaining := make([]slog.Attr, 0, len(attrs)-n)
	for _, a := range attrs {
		if h.isPrefix(a) {
			prefixes = append(prefixes, a.Value)
		} else {
			remaining = append(remaining, a)
		}
	}
	return prefixes, remaining
}

// isPrefix reports whether attr is a prefix value: a string under the prefix key.
// Other values under the key are left in place.
func (h *Handler) isPrefix(attr slog.Attr) bool {
	return attr.Key == h.opts.prefix && attr.Value.Kind() == slog.KindString
}
