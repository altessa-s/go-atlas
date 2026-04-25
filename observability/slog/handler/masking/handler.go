// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package masking

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/observability/slog/handler/internal/base"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// Handler wraps a [slog.Handler] to mask sensitive fields based on configuration.
// Field matching is case-insensitive. Safe for concurrent use.
type Handler struct {
	base.Base
	opts            *options
	lowercaseFields *coremaps.ImmutableMap[string, MaskFunc] // for case-insensitive matching
	patterns        []compiledPattern
	pathCache       sync.Map // string -> MaskFunc
	hasPatterns     bool
}

type compiledPattern struct {
	re   *regexp.Regexp
	mask MaskFunc
}

// NewHandler creates a masking handler wrapping inner. Panics if inner is nil.
//
// Example:
//
//	h := masking.NewHandler(slog.NewJSONHandler(os.Stdout, nil), masking.WithDefaults())
//	logger := slog.New(h)
func NewHandler(inner slog.Handler, opts ...Option) slog.Handler {
	o := newOptions(opts...)

	// Ensure we have a default mask
	if o.defaultMask == nil {
		o.defaultMask = FullMask()
	}

	h := &Handler{
		Base: base.NewBase(inner),
		opts: o,
	}

	// Pre-compute lowercase fields for case-insensitive matching
	if !o.caseSensitive {
		tmp := make(map[string]MaskFunc, len(o.fields))
		for k, v := range o.fields {
			tmp[corestrings.InternLowerString(k)] = v
		}
		h.lowercaseFields = coremaps.NewImmutableMap(tmp)
	}

	// Check if we have patterns
	if len(o.patterns) > 0 {
		h.patterns = make([]compiledPattern, 0, len(o.patterns))
		for _, p := range o.patterns {
			if p.Pattern == "" || p.Mask == nil {
				continue
			}
			re, err := regexp.Compile(p.Pattern)
			if err != nil {
				continue
			}
			h.patterns = append(h.patterns, compiledPattern{re: re, mask: p.Mask})
		}
	}
	h.hasPatterns = len(h.patterns) > 0

	return h
}

// Enabled reports whether the inner handler handles records at this level.
func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.Base.Enabled(ctx, level)
}

// Handle masks sensitive fields and passes the record to the inner handler.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	// Fast path: if no masking configured, skip processing
	if len(h.opts.fields) == 0 && !h.hasPatterns {
		return h.Inner().Handle(ctx, r)
	}

	// Clone the record to avoid modifying the original
	masked := base.CloneRecord(r)

	// Process and mask attributes
	groups := h.Groups()
	r.Attrs(func(a slog.Attr) bool {
		maskedAttr := h.maskAttribute(a, groups)
		masked.AddAttrs(maskedAttr)
		return true
	})

	return h.Inner().Handle(ctx, masked)
}

// WithAttrs returns a new Handler with the given attributes, masking sensitive ones.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Mask attributes before passing to inner handler
	groups := h.Groups()
	maskedAttrs := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		maskedAttrs = append(maskedAttrs, h.maskAttribute(attr, groups))
	}

	return &Handler{
		Base:            h.WithAttrsBase(maskedAttrs),
		opts:            h.opts,
		lowercaseFields: h.lowercaseFields,
		patterns:        h.patterns,
		hasPatterns:     h.hasPatterns,
	}
}

// WithGroup returns a new Handler with the given group name.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	return &Handler{
		Base:            h.WithGroupBase(name),
		opts:            h.opts,
		lowercaseFields: h.lowercaseFields,
		patterns:        h.patterns,
		hasPatterns:     h.hasPatterns,
	}
}

// maskAttribute recursively masks an attribute based on configuration
func (h *Handler) maskAttribute(attr slog.Attr, groups []string) slog.Attr {
	// Skip empty attributes
	if attr.Equal(slog.Attr{}) {
		return attr
	}

	// Check if this field should be masked
	fieldPath := h.buildFieldPath(groups, attr.Key)

	// Handle groups recursively
	if attr.Value.Kind() == slog.KindGroup {
		// Process group members
		groupAttrs := attr.Value.Group()
		maskedGroup := make([]slog.Attr, 0, len(groupAttrs))

		for _, ga := range groupAttrs {
			maskedGroup = append(maskedGroup, h.maskAttribute(ga, append(groups, attr.Key)))
		}

		return slog.Group(attr.Key, attrsToAny(maskedGroup)...)
	}

	// Check if field should be masked
	if mask := h.getMaskForField(attr.Key, fieldPath); mask != nil {
		return h.applyMask(attr, mask)
	}

	return attr
}

// getMaskForField uses pre-computed data for faster lookups
func (h *Handler) getMaskForField(fieldName, fieldPath string) MaskFunc {
	// Try cache first
	if cached, ok := h.pathCache.Load(fieldPath); ok {
		if mask, ok := cached.(MaskFunc); ok && mask != nil {
			return mask
		}
		// nil means no mask needed
		return nil
	}

	var mask MaskFunc

	// Check nested field paths first if enabled (more specific)
	if h.opts.maskNestedFields && fieldPath != fieldName {
		if h.opts.caseSensitive {
			mask = h.opts.fields[fieldPath]
		} else if h.lowercaseFields != nil {
			mask, _ = h.lowercaseFields.Get(corestrings.InternLowerString(fieldPath))
		}
	}

	// Check exact field matches if no nested match
	if mask == nil {
		if h.opts.caseSensitive {
			mask = h.opts.fields[fieldName]
		} else if h.lowercaseFields != nil {
			mask, _ = h.lowercaseFields.Get(corestrings.InternLowerString(fieldName))
		}
	}

	// Check pattern matches (currently not working in original implementation)
	if mask == nil && h.hasPatterns {
		// Prefer the full field path when nested masking is enabled.
		target := fieldName
		if h.opts.maskNestedFields && fieldPath != fieldName {
			target = fieldPath
		}

		for i := range h.patterns {
			if h.patterns[i].re.MatchString(target) {
				mask = h.patterns[i].mask
				break
			}
		}
	}

	// Cache the result (including nil for no mask)
	h.pathCache.Store(fieldPath, mask)

	return mask
}

// buildFieldPath creates the full path for nested fields
func (h *Handler) buildFieldPath(groups []string, field string) string {
	if !h.opts.maskNestedFields || len(groups) == 0 {
		return field
	}

	// Use strings.Builder for better performance
	var sb strings.Builder
	// Pre-calculate capacity
	capacity := len(field)
	for _, g := range groups {
		capacity += len(g) + 1
	}
	sb.Grow(capacity)

	// Build path
	for _, g := range groups {
		sb.WriteString(g)
		sb.WriteByte('.')
	}
	sb.WriteString(field)

	return sb.String()
}

// applyMask applies the masking function to an attribute value
func (h *Handler) applyMask(attr slog.Attr, mask MaskFunc) slog.Attr {
	switch attr.Value.Kind() {
	case slog.KindString:
		return slog.String(attr.Key, mask(attr.Value.String()))

	case slog.KindInt64, slog.KindUint64, slog.KindFloat64, slog.KindBool:
		// Convert to string, mask, and keep as string
		str := fmt.Sprint(attr.Value.Any())
		return slog.String(attr.Key, mask(str))

	case slog.KindDuration:
		return slog.String(attr.Key, mask(attr.Value.Duration().String()))

	case slog.KindTime:
		return slog.String(attr.Key, mask(attr.Value.Time().String()))

	case slog.KindAny:
		// Arrays are masked as whole (current implementation limitation)
		str := fmt.Sprint(attr.Value.Any())
		return slog.String(attr.Key, mask(str))

	default:
		// For unknown types, convert to string and mask
		str := fmt.Sprint(attr.Value.Any())
		return slog.String(attr.Key, mask(str))
	}
}

// attrsToAny converts attributes to []any for slog.Group
func attrsToAny(attrs []slog.Attr) []any {
	result := make([]any, len(attrs))
	for i, attr := range attrs {
		result[i] = attr
	}
	return result
}
