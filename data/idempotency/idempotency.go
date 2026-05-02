// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/data/idempotency/storages"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// serializedState is the on-the-wire shape Keeper writes through the
// underlying Storage. The Nonce field is added on every AttemptLock to
// guarantee that two consecutive in-progress writes for the same key
// produce distinct bytes — required for byte-equality CAS guards in
// the memory and redis backends. NATS uses revisions and ignores the
// nonce. LockedAt records when the InProgress wire was written; Keeper
// uses it to detect orphan locks left behind by crashed holders. The
// public [State] type does NOT carry these fields.
type serializedState struct {
	Status   storages.Status `json:"status"`
	Data     any             `json:"data,omitempty"`
	Nonce    string          `json:"nonce,omitempty"`
	LockedAt time.Time       `json:"locked_at,omitzero"`
}

// Re-export types from storages for convenience
type (
	Status = storages.Status
	State  = storages.State
)

const (
	// StatusInProgress indicates the request is currently being processed.
	StatusInProgress = storages.StatusInProgress
	// StatusSuccess indicates the request was completed successfully.
	StatusSuccess = storages.StatusSuccess
)

// ErrEmptyKey is returned by [Keeper] when an empty idempotency key is
// supplied. Re-exported from [storages.ErrEmptyKey] for convenience —
// `errors.Is(err, idempotency.ErrEmptyKey)` matches both layers.
var ErrEmptyKey = storages.ErrEmptyKey

// ErrLockStolen is returned by [Keeper.Complete] when the lock has
// been taken over by another holder between AttemptLock and Complete
// (typically because the lock TTL expired during processing).
// Re-exported from [storages.ErrLockStolen].
var ErrLockStolen = storages.ErrLockStolen

// ErrMissingLockState is returned by [Keeper.Complete] when the caller
// passes a nil lock state. The state returned by AttemptLock carries
// the CAS token; passing nil would silently disable the stolen-lock
// guard. Re-exported from [storages.ErrMissingLockState].
var ErrMissingLockState = storages.ErrMissingLockState

// AttemptLockOpts customizes a single AttemptLockWithOpts call. Both
// fields are optional; zero values fall back to Keeper / backend
// defaults.
type AttemptLockOpts struct {
	// LockTTL overrides the storage backend's configured TTL for this
	// lock. Pass <= 0 to use the backend default.
	LockTTL time.Duration

	// MaxLockDuration overrides the Keeper's configured threshold for
	// orphan-lock reclaim. When AttemptLock observes an existing
	// InProgress entry older than this, it issues a CAS-steal to take
	// ownership. Pass <= 0 to use the Keeper default
	// ([DefaultMaxLockDuration]). Set the Keeper-wide value via
	// [WithMaxLockDuration].
	MaxLockDuration time.Duration
}

// Idempotency defines the interface for idempotency key operations.
// Implementations must be safe for concurrent use.
type Idempotency interface {
	// AttemptLock tries to acquire a lock for the given key using the
	// backend's configured TTL and the Keeper's configured
	// MaxLockDuration. Equivalent to
	// AttemptLockWithOpts(ctx, key, AttemptLockOpts{}).
	AttemptLock(ctx context.Context, key string) (bool, *storages.State, error)

	// AttemptLockWithOpts is like [Idempotency.AttemptLock] but
	// applies the per-call overrides in opts. Use this when a
	// particular key needs a different lock lifetime (e.g. short-lived
	// OTP tokens vs long-running webhook processing) or a different
	// orphan-reclaim threshold.
	AttemptLockWithOpts(ctx context.Context, key string, opts AttemptLockOpts) (bool, *storages.State, error)

	// Complete marks the key as successfully processed. The lockState
	// must be the *State returned by [Idempotency.AttemptLock] —
	// its embedded CAS token gates the write. Returns
	// [ErrLockStolen] when the lock has been taken over by another
	// holder since AttemptLock, [ErrMissingLockState] when lockState
	// is nil.
	Complete(ctx context.Context, key string, data any, lockState *storages.State) error

	// Delete removes the key from storage (e.g. on failure).
	Delete(ctx context.Context, key string) error
}

// Keeper provides duplicate request detection using idempotency keys.
// It supports multiple storage backends for distributed systems.
type Keeper struct {
	storage storages.Storage
	opts    *options
	metrics *keeperMetrics
}

var _ Idempotency = (*Keeper)(nil)

// New creates a new Keeper with the specified storage backend.
//
// When the caller does not pass [WithMaxLockDuration], New emits a
// single slog.Warn so service owners notice they are accepting the
// package-level default ([DefaultMaxLockDuration]) for orphan-lock
// reclaim. Pass any explicit positive value — including
// `WithMaxLockDuration(DefaultMaxLockDuration)` — to silence the
// warning.
//
// Example:
//
//	keeper := idempotency.New(memoryStorage)
func New(storage storages.Storage, opt ...Option) *Keeper {
	options := newOptions(opt...)

	if options.serializer == nil {
		options.serializer = &serializer.JSON{}
	}

	if options.maxLockDuration == 0 {
		options.logger.Warn(
			"idempotency: using default MaxLockDuration; review for your service",
			slog.Duration("default", DefaultMaxLockDuration),
			slog.String("fix", "pass idempotency.WithMaxLockDuration(d) to acknowledge or override"),
		)
		options.maxLockDuration = DefaultMaxLockDuration
	}

	return &Keeper{
		storage: storage,
		opts:    options,
		metrics: newKeeperMetrics(options.collector),
	}
}

// AttemptLock tries to acquire a lock for the given key using the
// backend's configured TTL and the Keeper's configured
// MaxLockDuration. An empty key returns [ErrEmptyKey].
func (i *Keeper) AttemptLock(ctx context.Context, key string) (bool, *storages.State, error) {
	return i.AttemptLockWithOpts(ctx, key, AttemptLockOpts{})
}

// AttemptLockWithOpts is like [Keeper.AttemptLock] but applies the
// per-call overrides in opts. See [AttemptLockOpts] for field
// semantics.
//
// On collision with an existing InProgress entry whose LockedAt
// timestamp is older than the resolved MaxLockDuration, AttemptLock
// performs a CAS-steal via [storages.Storage.Steal] to reclaim the
// orphaned lock. The returned *State carries a fresh lock token in
// that case, so the caller proceeds as the legitimate holder.
func (i *Keeper) AttemptLockWithOpts(ctx context.Context, key string, opts AttemptLockOpts) (bool, *storages.State, error) {
	if key == "" {
		return false, nil, ErrEmptyKey
	}

	// Embed a per-attempt nonce so two consecutive lock attempts on
	// the same key produce distinct serialized bytes. Storage backends
	// that CAS on byte equality (memory, redis) need this to tell two
	// holders apart. NATS uses revisions and ignores the field.
	wire := serializedState{
		Status:   storages.StatusInProgress,
		Nonce:    uuid.NewString(),
		LockedAt: time.Now(),
	}

	val, err := i.opts.serializer.Serialize(wire)
	if err != nil {
		return false, nil, coreerrs.WrapOperation(err, "serialize state")
	}

	ok, existingVal, lockToken, err := i.storage.AttemptLockWithTTL(ctx, key, val, opts.LockTTL)
	if err != nil {
		i.metrics.errors.Inc()
		return false, nil, err
	}

	if ok {
		i.metrics.locksAcquired.Inc()
		// Carry the CAS token on the State so Complete can validate
		// our claim later. The State itself isn't serialized at this
		// point; we hand the freshly-built object back to the caller.
		acquired := &storages.State{Status: storages.StatusInProgress}
		acquired.SetLockToken(lockToken)
		return true, acquired, nil
	}

	// Lock failed, key exists. Deserialize existing state via the
	// wire shape and strip the nonce/LockedAt before handing back to
	// the caller — they shouldn't need to know about CAS plumbing.
	var existing serializedState
	if err := i.opts.serializer.Deserialize(existingVal, &existing); err != nil {
		i.metrics.errors.Inc()
		return false, nil, coreerrs.WrapOperation(err, "deserialize existing state")
	}

	// Orphan-steal: an InProgress entry whose LockedAt is older than
	// the resolved MaxLockDuration belongs to a crashed (or hung)
	// holder. CAS-steal it so retries don't get stuck behind an
	// abandoned lock until the bucket TTL expires.
	if existing.Status == storages.StatusInProgress {
		if maxLock := i.resolveMaxLockDuration(opts); maxLock > 0 &&
			!existing.LockedAt.IsZero() &&
			time.Since(existing.LockedAt) > maxLock {
			newToken, stealErr := i.storage.Steal(ctx, key, existingVal, val)
			switch {
			case stealErr == nil:
				i.metrics.locksAcquired.Inc()
				stolen := &storages.State{Status: storages.StatusInProgress}
				stolen.SetLockToken(newToken)
				return true, stolen, nil
			case errors.Is(stealErr, storages.ErrLockStolen):
				// Race: someone else stole or the entry vanished
				// between our Get and Steal. Fall through and surface
				// the previously observed state — caller will retry.
			default:
				i.metrics.errors.Inc()
				return false, nil, stealErr
			}
		}
	}

	i.metrics.locksDenied.Inc()
	return false, &storages.State{Status: existing.Status, Data: existing.Data}, nil
}

// resolveMaxLockDuration returns the per-call override when set,
// otherwise the Keeper's configured value.
func (i *Keeper) resolveMaxLockDuration(opts AttemptLockOpts) time.Duration {
	if opts.MaxLockDuration > 0 {
		return opts.MaxLockDuration
	}
	return i.opts.maxLockDuration
}

// Complete marks the key as successfully processed. The lockState
// must be the *State returned by [Keeper.AttemptLock] — its embedded
// CAS token gates the write. An empty key returns [ErrEmptyKey];
// a nil lockState returns [ErrMissingLockState]; a stolen lock
// returns [ErrLockStolen].
func (i *Keeper) Complete(ctx context.Context, key string, data any, lockState *storages.State) error {
	if key == "" {
		return ErrEmptyKey
	}
	if lockState == nil {
		return ErrMissingLockState
	}

	state := storages.State{
		Status: storages.StatusSuccess,
		Data:   data,
	}

	val, err := i.opts.serializer.Serialize(state)
	if err != nil {
		return coreerrs.WrapOperation(err, "serialize state")
	}

	if err := i.storage.Complete(ctx, key, val, lockState.LockToken()); err != nil {
		i.metrics.errors.Inc()
		return err
	}
	i.metrics.completions.Inc()
	return nil
}

// Delete removes the key from storage (e.g. on failure). An empty key
// returns [ErrEmptyKey].
func (i *Keeper) Delete(ctx context.Context, key string) error {
	if key == "" {
		return ErrEmptyKey
	}
	if err := i.storage.Delete(ctx, key); err != nil {
		i.metrics.errors.Inc()
		return err
	}
	i.metrics.deletions.Inc()
	return nil
}
