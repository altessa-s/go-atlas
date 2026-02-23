// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package propagation

import (
	"context"
	"maps"
	"net/http"
	"slices"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

// TextMapCarrier is the interface for injecting and extracting trace context.
// See [HeaderCarrier] for http.Header and [MapCarrier] for map[string]string.
type TextMapCarrier interface {
	// Get returns the value for a key.
	// Returns empty string if the key does not exist.
	Get(key string) string

	// Set stores a key-value pair.
	Set(key, value string)

	// Keys returns all keys in the carrier.
	Keys() []string
}

// TextMapPropagator propagates trace context in text format.
// See the tracecontext sub-package for the W3C Trace Context implementation.
// Use [CompositePropagator] to combine multiple propagators.
type TextMapPropagator interface {
	// Inject injects trace context into a carrier.
	// The carrier is typically HTTP headers or messaging metadata.
	Inject(ctx context.Context, carrier TextMapCarrier)

	// Extract extracts trace context from a carrier.
	// Returns a new context with the extracted trace context.
	Extract(ctx context.Context, carrier TextMapCarrier) context.Context

	// Fields returns the list of header keys used by this propagator.
	// This is used to determine which headers to forward.
	Fields() []string
}

// HeaderCarrier adapts [http.Header] to [TextMapCarrier].
type HeaderCarrier http.Header

// Get implements TextMapCarrier.
func (h HeaderCarrier) Get(key string) string {
	return http.Header(h).Get(key)
}

// Set implements TextMapCarrier.
func (h HeaderCarrier) Set(key, value string) {
	http.Header(h).Set(key, value)
}

// Keys implements TextMapCarrier.
func (h HeaderCarrier) Keys() []string {
	return slices.Collect(maps.Keys(h))
}

// MapCarrier adapts map[string]string to [TextMapCarrier].
type MapCarrier map[string]string

// Get implements TextMapCarrier.
func (m MapCarrier) Get(key string) string {
	return m[key]
}

// Set implements TextMapCarrier.
func (m MapCarrier) Set(key, value string) {
	m[key] = value
}

// Keys implements TextMapCarrier.
func (m MapCarrier) Keys() []string {
	return slices.Collect(maps.Keys(m))
}

// CompositePropagator combines multiple [TextMapPropagator] instances.
// It calls all propagators in order for injection and extraction.
// Not safe for concurrent modification after construction; concurrent
// [CompositePropagator.Inject] and [CompositePropagator.Extract] calls are safe.
type CompositePropagator struct {
	propagators []TextMapPropagator
}

// NewCompositePropagator creates a new composite propagator.
func NewCompositePropagator(propagators ...TextMapPropagator) *CompositePropagator {
	return &CompositePropagator{propagators: propagators}
}

// Inject implements TextMapPropagator.
func (c *CompositePropagator) Inject(ctx context.Context, carrier TextMapCarrier) {
	for _, p := range c.propagators {
		p.Inject(ctx, carrier)
	}
}

// Extract implements TextMapPropagator.
func (c *CompositePropagator) Extract(ctx context.Context, carrier TextMapCarrier) context.Context {
	for _, p := range c.propagators {
		ctx = p.Extract(ctx, carrier)
	}
	return ctx
}

// Fields implements TextMapPropagator.
func (c *CompositePropagator) Fields() []string {
	fields := make([]string, 0, len(c.propagators))
	for _, p := range c.propagators {
		fields = append(fields, p.Fields()...)
	}
	return coreslices.Deduplicate(fields)
}

// Ensure implementations satisfy the interface.
var (
	_ TextMapCarrier    = HeaderCarrier{}
	_ TextMapCarrier    = MapCarrier{}
	_ TextMapPropagator = (*CompositePropagator)(nil)
)
