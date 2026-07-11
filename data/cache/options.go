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

// KeyNamespaceFunc derives a namespace prefix from the request context so cache
// entries and in-flight singleflight de-duplication are isolated per
// tenant/subject. It is the recommended way to prevent cross-tenant fan-in when
// callers might otherwise build the same bare key for different principals.
// Returning "" disables prefixing for that call, which is the backward-compatible
// default (no namespace configured).
type KeyNamespaceFunc func(context.Context) string

// DefaultTTL is the default time-to-live for cache items (1 hour).
const DefaultTTL = 1 * time.Hour

// DefaultMaxConcurrentFallbacks bounds the number of fallback functions
// the cache will run in parallel, regardless of how many distinct keys
// arrive at GetWithFallback. Without this bound an attacker driving
// arbitrary cache keys (one per request) can launch an unbounded number
// of slow downstream calls in parallel — singleflight only dedupes
// IDENTICAL keys, so distinct attacker-chosen keys all pass through.
//
// 1024 is high enough that legitimate traffic never queues in practice
// (singleflight already collapses repeats of hot keys), but low enough
// that the worst-case memory footprint of in-flight fallbacks is
// bounded and the downstream gets reasonable backpressure under attack.
const DefaultMaxConcurrentFallbacks = 1024

// options contains Cache configuration.
type options struct {
	ttl                    time.Duration `optgen:"default=DefaultTTL"`
	negativeTtl            time.Duration // zero = disabled
	maxConcurrentFallbacks int           `optgen:"default=DefaultMaxConcurrentFallbacks"`
	serializer             serializer.Serializer
	collector              metrics.Collector `optgen:"notnil"`
	name                   string
	// keyNamespace, when set, isolates cache entries and singleflight de-dup by a
	// per-context namespace (typically tenant/subject). Nil = no prefixing.
	keyNamespace KeyNamespaceFunc `optgen:"default=nil"`
}
