// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package colorized

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"sync"

	"github.com/fatih/color"
)

var noColorMu sync.Mutex

// Handler implements [slog.Handler] with colorized terminal output.
// It uses lazy evaluation, lock-free caches, and buffer pooling for performance.
// Safe for concurrent use.
type Handler struct {
	mu   *sync.Mutex // Protects writes to the shared writer; shared across clones
	w    io.Writer
	opts *options

	// Lazy-evaluated values for performance
	lazyColorMap    *lazyColorMap    // Defers color map conversion until first use
	lazyGroupPrefix *lazyGroupPrefix // Builds group prefix only when needed
	minLevel        slog.Level       // Pre-computed for fast Enabled checks

	// Handler state for WithAttrs/WithGroup
	groups []string
	attrs  []slog.Attr
}

// NewHandler creates a colorized handler that writes to the provided io.Writer.
// Panics if w is nil. Uses default options if none provided.
//
// Example:
//
//	h := colorized.NewHandler(os.Stdout)
//	logger := slog.New(h)
func NewHandler(w io.Writer, opts ...Option) slog.Handler {
	if w == nil {
		panic("colorized: writer cannot be nil")
	}

	o := newOptions(opts...)

	// Create handler with lazy evaluation for optimal performance.
	// Expensive operations like color map conversion and group prefix building
	// are deferred until they're actually needed.
	h := &Handler{
		mu:              new(sync.Mutex),
		w:               w,
		opts:            o,
		minLevel:        o.level.Level(),
		attrs:           make([]slog.Attr, 0),
		groups:          make([]string, 0),
		lazyColorMap:    newLazyColorMap(o.attributeColors),
		lazyGroupPrefix: newLazyGroupPrefix(nil),
	}

	// Apply NoColor setting globally if specified
	if o.noColor {
		noColorMu.Lock()
		color.NoColor = true
		noColorMu.Unlock()
	}

	return h
}

// Enabled reports whether the handler handles records at the given level.
func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minLevel
}

// Handle formats and writes a log record with ANSI color codes.
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	// Get buffer from pool
	buf := newBuffer()
	defer buf.Free()

	// Format the record
	h.formatRecord(buf, r)

	// Write to output (thread-safe)
	h.mu.Lock()
	_, err := h.w.Write(buf.finalize())
	h.mu.Unlock()

	return err
}

// WithAttrs returns a new Handler with the given attributes added.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}

	// Clone handler
	h2 := h.clone()

	// Process attributes through ReplaceAttr if configured
	for _, attr := range attrs {
		if h.opts.replaceAttr != nil {
			attr = h.opts.replaceAttr(h2.groups, attr)
		}
		// Skip empty attributes
		if !attr.Equal(slog.Attr{}) {
			h2.attrs = append(h2.attrs, attr)
		}
	}

	return h2
}

// WithGroup returns a new Handler with the given group name prepended to keys.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	h2 := h.clone()
	h2.groups = append(h2.groups, name)
	// Group prefix will be built lazily when needed

	return h2
}

// Internal methods

// clone creates a copy of the handler
func (h *Handler) clone() *Handler {
	return &Handler{
		mu:              h.mu, // Share the writer mutex so all clones serialize writes
		w:               h.w,
		opts:            h.opts,
		lazyColorMap:    h.lazyColorMap, // Share the lazy color map
		minLevel:        h.minLevel,
		groups:          slices.Clone(h.groups),
		lazyGroupPrefix: newLazyGroupPrefix(h.groups), // Create new lazy prefix
		attrs:           slices.Clone(h.attrs),
	}
}

// formatRecord formats a complete log record
func (h *Handler) formatRecord(buf *buffer, r slog.Record) {
	// 1. Format time if present
	if !r.Time.IsZero() {
		buf.formatBuiltinAttr(
			slog.Time(slog.TimeKey, r.Time.Round(0)),
			h.opts.timeFormat,
		)
	}

	// 2. Format level
	buf.formatBuiltinAttr(
		slog.Int64(slog.LevelKey, int64(r.Level)),
		"",
	)

	// 3. Collect all attributes
	attrs := h.collectAttributes(r)

	// 4. Extract and format prefix if configured
	if h.opts.prefixAttributeKey != "" {
		attrs = h.formatPrefix(buf, attrs)
	}

	// 5. Format message
	buf.formatBuiltinAttr(slog.String(slog.MessageKey, r.Message), "")

	// 6. Prepare source info if configured.
	// Source extraction is expensive, so we defer it until formatting time.
	var lazySource *lazySource
	if h.opts.addSource && r.PC != 0 {
		lazySource = newLazySource(r.PC)
	}

	// 7. Format all remaining attributes
	if len(attrs) > 0 || lazySource != nil {
		// Get group prefix lazily
		groupPrefix := h.lazyGroupPrefix.get()

		// Get color map lazily
		colorMap := h.lazyColorMap.get()

		// Format attributes
		buf.formatAttributes(attrs, groupPrefix, colorMap)

		// Add source attribute if needed
		if lazySource != nil {
			if len(attrs) > 0 {
				buf.writeBytes(spaceBytes)
			}
			sourceAttr := lazySource.get()
			buf.formatAttribute(sourceAttr, groupPrefix, colorMap)
		}
	}
}

// collectAttributes gathers all attributes from handler and record
func (h *Handler) collectAttributes(r slog.Record) []slog.Attr {
	// Pre-allocate with capacity
	attrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())

	// Add handler attributes
	attrs = append(attrs, h.attrs...)

	// Add record attributes with ReplaceAttr processing
	r.Attrs(func(a slog.Attr) bool {
		if h.opts.replaceAttr != nil {
			a = h.opts.replaceAttr(h.groups, a)
		}
		if !a.Equal(slog.Attr{}) {
			attrs = append(attrs, a)
		}
		return true
	})

	return attrs
}

// formatPrefix extracts and formats prefix attributes
func (h *Handler) formatPrefix(buf *buffer, attrs []slog.Attr) []slog.Attr {
	// Find prefix attributes
	var prefixValue string
	remaining := make([]slog.Attr, 0, len(attrs))

	for _, attr := range attrs {
		if attr.Key == h.opts.prefixAttributeKey && attr.Value.Kind() == slog.KindString {
			if prefixValue == "" {
				prefixValue = attr.Value.String()
			}
		} else {
			remaining = append(remaining, attr)
		}
	}

	// Format prefix if found
	if prefixValue != "" {
		// Get color map lazily only if we have a prefix
		colorMap := h.lazyColorMap.get()
		colors := colorMap[h.opts.prefixAttributeKey]
		if len(colors) == 0 {
			colors = []color.Attribute{color.BgCyan}
		}
		buf.writeColorized(prefixValue, colors...)
		buf.writeBytes(spaceBytes)
	}

	return remaining
}
