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

// KeyNamespaceFunc derives a namespace from the request context so idempotency
// keys are isolated per tenant/subject. The namespace is one component of the
// storage key [ComposeKey] builds, so namespaces and keys never collide
// whatever bytes they hold. Without it, every key lives in the empty
// namespace: two tenants using the same idempotency key collide, and one can
// replay the other's completed response. Returning "" selects the empty
// namespace for that call, the same one a nil KeyNamespaceFunc uses; the key
// is still composed, never stored unchanged.
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
	// namespace (typically tenant/subject). Nil = the empty namespace.
	keyNamespace KeyNamespaceFunc `optgen:"default=nil"`
	// owner, when set, records the caller's owner label with each lock.
	// Without replayAuthorizer, a caller with another label is refused the
	// stored state. Nil = no owner recorded.
	owner OwnerFunc `optgen:"default=nil"`
	// replayAuthorizer, when set, decides every replay from the recorded
	// owner label, instead of comparing labels.
	replayAuthorizer ReplayAuthorizer `optgen:"default=nil"`
}

// OwnerFunc returns the owner label recorded with each lock the caller in
// ctx takes: a subject, or any access attributes the service needs to
// decide visibility later (a tenant and a subject, an encoded resource
// owner). The label is opaque to the Keeper. "" records no owner.
type OwnerFunc func(ctx context.Context) string

// ReplayAuthorizer decides whether the caller in ctx may see the state of a
// key recorded with owner (the [OwnerFunc] label, "" when none was
// recorded). When set, it is called for every replay instead of comparing
// owner labels: nil lets the caller see the record, an error is returned to
// the caller wrapped in ErrOwnerMismatch — return the service's NotFound
// error to hide the record, and gRPC transports answer with its status.
// Taking over an orphaned lock still requires the same owner label.
type ReplayAuthorizer func(ctx context.Context, owner string) error
