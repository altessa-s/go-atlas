// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"context"
	"time"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// KeyNamespaceFunc derives a namespace from context. Namespace/key pairs are
// encoded with a length prefix; an empty namespace is distinct from every
// non-empty namespace. Nil disables namespacing.
type KeyNamespaceFunc func(context.Context) string

// DefaultTTL is the default time-to-live for cache items (1 hour).
const DefaultTTL = 1 * time.Hour

// DefaultMaxConcurrentFallbacks bounds distinct active fallback computations.
// New keys at capacity return ErrOverloaded without creating background work.
// Calls sharing an active key can still join it. Non-positive values disable
// this admission limit.
const DefaultMaxConcurrentFallbacks = 1024

// DefaultFallbackTimeout bounds the shared fallback and cache write context.
const DefaultFallbackTimeout = 15 * time.Second

// options contains Cache configuration.
type options struct {
	ttl                    time.Duration `optgen:"default=DefaultTTL"`
	fallbackTimeout        time.Duration `optgen:"default=DefaultFallbackTimeout"`
	negativeTtl            time.Duration // zero = disabled
	maxConcurrentFallbacks int           `optgen:"default=DefaultMaxConcurrentFallbacks"`
	serializer             serializer.Serializer
	collector              metrics.Collector `optgen:"notnil"`
	name                   string
	// keyNamespace, when set, isolates cache entries and singleflight de-dup by a
	// per-context namespace (typically tenant/subject). Nil = no prefixing.
	keyNamespace KeyNamespaceFunc `optgen:"default=nil"`
	// failOpen, set by [WithFailOpen], degrades storage and decode failures to
	// cache misses instead of returning them.
	failOpen bool `opt:"-"`
	// ioTimeout, set by [WithFailOpen], bounds every storage call; zero adds no bound.
	ioTimeout time.Duration `opt:"-"`
	// isolatedResults, set by [WithIsolatedResults], makes collapsed callers
	// share serialized bytes and decode their own copy.
	isolatedResults bool `opt:"-"`
}

// WithFailOpen makes the cache an optimization that never fails a request on
// its own: a storage error or timeout on a read is a miss, a stored value that
// cannot be decoded is a miss (and is deleted best-effort), a failed write is
// ignored, and a fallback rejected with [ErrOverloaded] runs directly in the
// caller. Each such event is counted in the errors and fail_open metrics.
// Fallback errors, [ErrMissing] and negative caching are unchanged, and a
// caller's own context cancellation is still returned.
//
// A positive ioTimeout bounds every storage Get, GetMany and Save call; zero
// adds no bound beyond the caller's context. Negative values are treated as zero.
func WithFailOpen(ioTimeout time.Duration) Option {
	return func(o *options) {
		o.failOpen = true
		o.ioTimeout = max(ioTimeout, 0)
	}
}

// WithIsolatedResults makes callers collapsed onto one fallback by
// [Cache.GetWithFallback] and [GetWithFallbackT] share the serialized bytes
// rather than the loaded value: every caller, the one that ran the fallback
// included, decodes its own copy, so no slice, map or pointer is shared
// between callers. It costs one decode per caller on the miss path.
func WithIsolatedResults() Option {
	return func(o *options) {
		o.isolatedResults = true
	}
}
