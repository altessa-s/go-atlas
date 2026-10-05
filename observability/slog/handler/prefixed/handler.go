// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package prefixed

import (
	"context"
	"log/slog"

	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/observability/slog/handler/internal/base"

	stdSlices "slices"
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
	// Treat h.prefixes as immutable. Only allocate when we need to add/merge prefixes.
	prefixes := h.prefixes
	if r.NumAttrs() > 0 {
		attrs := make([]slog.Attr, 0, r.NumAttrs())
		r.Attrs(func(a slog.Attr) bool {
			attrs = append(attrs, a)
			return true
		})

		p, nattrs := h.extractPrefixes(attrs)
		if len(p) > 0 {
			r = slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
			r.AddAttrs(nattrs...)
			prefixes = p
		}
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

// extractPrefixes returns prefix values and remaining non-prefix attributes.
func (h *Handler) extractPrefixes(attrs []slog.Attr) ([]slog.Value, []slog.Attr) {
	prefixes := h.prefixes

	if pfx := stdSlices.Collect(slices.Filter(attrs, h.isPrefix)); len(pfx) > 0 {
		prefixes = make([]slog.Value, 0, len(h.prefixes)+len(pfx))
		prefixes = append(prefixes, h.prefixes...)

		prefixes = append(prefixes, stdSlices.Collect(slices.Map(pfx, func(attr slog.Attr) slog.Value {
			return attr.Value
		}))...)

		attrs = stdSlices.Collect(slices.Filter(attrs, func(attr slog.Attr) bool {
			return !h.isPrefix(attr)
		}))
	}

	return prefixes, attrs
}

// isPrefix reports whether attr is a prefix value: a string under the prefix key.
// Other values under the key are left in place.
func (h *Handler) isPrefix(attr slog.Attr) bool {
	return attr.Key == h.opts.prefix && attr.Value.Kind() == slog.KindString
}
