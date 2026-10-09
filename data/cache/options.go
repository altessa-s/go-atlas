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
}
