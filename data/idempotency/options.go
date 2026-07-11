// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"context"
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// KeyNamespaceFunc derives a namespace prefix from the request context so
// idempotency keys are isolated per tenant/subject. Without it, a bare key is
// global: two tenants using the same idempotency key collide, and one can replay
// the other's completed response. Returning "" disables prefixing for that call,
// which is the backward-compatible default (no namespace configured).
type KeyNamespaceFunc func(context.Context) string

// DefaultMaxLockDuration is the default threshold for treating an
// in-progress lock as orphaned. AttemptLock callers that observe a
// stale InProgress entry older than this attempt a CAS-steal.
const DefaultMaxLockDuration = 5 * time.Minute

// options contains Keeper configuration.
type options struct {
	logger     *slog.Logger
	serializer serializer.Serializer `optgen:"notnil"`
	collector  metrics.Collector     `optgen:"notnil"`
	// maxLockDuration is the threshold for orphan-lock detection.
	// On AttemptLock collision, if the existing InProgress entry is
	// older than this, Keeper attempts a CAS-steal. The field is
	// intentionally left without an [optgen] default: when [Keeper.New]
	// observes a zero value (i.e. the caller did not invoke
	// [WithMaxLockDuration]) it logs a warning and normalizes the
	// value to [DefaultMaxLockDuration]. Service owners silence the
	// warning by passing any explicit positive value — including
	// `WithMaxLockDuration(DefaultMaxLockDuration)` to acknowledge the
	// default. Disabling orphan-reclaim entirely is a Keeper-internal
	// concern (set the field to zero post-construction in tests).
	maxLockDuration time.Duration
	// keyNamespace, when set, isolates idempotency keys by a per-context
	// namespace (typically tenant/subject). Nil = no prefixing.
	keyNamespace KeyNamespaceFunc `optgen:"default=nil"`
}
