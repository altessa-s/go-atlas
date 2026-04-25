// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// lazy.go provides lazy evaluation for source info, group prefix, and color maps.

package colorized

import (
	"log/slog"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/fatih/color"
)

// lazySource defers source information computation until needed
type lazySource struct {
	pc       uintptr
	computed atomic.Bool
	mu       sync.Mutex
	attr     slog.Attr
}

// newLazySource creates a new lazy source evaluator
func newLazySource(pc uintptr) *lazySource {
	return &lazySource{pc: pc}
}

// get returns the source attribute, computing it only once
func (s *lazySource) get() slog.Attr {
	// Fast path: already computed
	if s.computed.Load() {
		return s.attr
	}

	// Slow path: compute source info
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring lock
	if s.computed.Load() {
		return s.attr
	}

	// Compute source information
	fs := runtime.CallersFrames([]uintptr{s.pc})
	f, _ := fs.Next()

	s.attr = slog.Group(slog.SourceKey,
		slog.String("file", f.File),
		slog.Int("line", f.Line),
		slog.String("func", f.Function),
	)

	s.computed.Store(true)
	return s.attr
}

// lazyGroupPrefix builds group prefix on first access
type lazyGroupPrefix struct {
	groups   []string
	computed atomic.Bool
	prefix   atomic.Pointer[string]
}

// newLazyGroupPrefix creates a new lazy group prefix builder
func newLazyGroupPrefix(groups []string) *lazyGroupPrefix {
	return &lazyGroupPrefix{groups: slices.Clone(groups)}
}

// get returns the group prefix, building it only once
func (p *lazyGroupPrefix) get() string {
	// Fast path: already computed
	if ptr := p.prefix.Load(); ptr != nil {
		return *ptr
	}

	// Check if we need to compute
	if !p.computed.Load() && p.computed.CompareAndSwap(false, true) {
		prefix := buildGroupPrefix(p.groups)
		p.prefix.Store(&prefix)
		return prefix
	}

	// Race condition: another goroutine is computing
	// Spin-wait for result
	for {
		if ptr := p.prefix.Load(); ptr != nil {
			return *ptr
		}
		runtime.Gosched()
	}
}

// buildGroupPrefix constructs the dot-separated group prefix
func buildGroupPrefix(groups []string) string {
	if len(groups) == 0 {
		return ""
	}

	// Pre-calculate size to avoid reallocations
	size := len(groups) - 1 // dots
	for _, g := range groups {
		size += len(g)
	}

	// Build prefix efficiently
	b := make([]byte, 0, size+1)
	for i, g := range groups {
		if i > 0 {
			b = append(b, '.')
		}
		b = append(b, g...)
	}
	b = append(b, '.')

	return string(b)
}

// lazyColorMap converts color integers to attributes on first access
type lazyColorMap struct {
	intColors map[string][]int
	computed  atomic.Bool
	colorMap  atomic.Pointer[map[string][]color.Attribute]
}

// newLazyColorMap creates a new lazy color map converter
func newLazyColorMap(intColors map[string][]int) *lazyColorMap {
	return &lazyColorMap{intColors: intColors}
}

// get returns the converted color map, building it only once
func (m *lazyColorMap) get() map[string][]color.Attribute {
	// Fast path: already computed
	if ptr := m.colorMap.Load(); ptr != nil {
		return *ptr
	}

	// Check if we need to compute
	if !m.computed.Load() && m.computed.CompareAndSwap(false, true) {
		colorMap := m.buildColorMap()
		m.colorMap.Store(&colorMap)
		return colorMap
	}

	// Race condition: another goroutine is computing
	// Spin-wait for result
	for {
		if ptr := m.colorMap.Load(); ptr != nil {
			return *ptr
		}
		runtime.Gosched()
	}
}

// buildColorMap converts integer colors to color.Attribute slices
func (m *lazyColorMap) buildColorMap() map[string][]color.Attribute {
	if len(m.intColors) == 0 {
		return nil
	}

	colorMap := make(map[string][]color.Attribute, len(m.intColors))
	for key, colors := range m.intColors {
		attrs := make([]color.Attribute, len(colors))
		for i, c := range colors {
			attrs[i] = color.Attribute(c)
		}
		colorMap[key] = attrs
	}

	return colorMap
}
