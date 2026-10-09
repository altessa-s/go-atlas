// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
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
	Status      storages.Status `json:"status"`
	Data        any             `json:"data,omitempty"`
	Nonce       string          `json:"nonce,omitempty"`
	LockedAt    time.Time       `json:"locked_at,omitzero"`
	Fingerprint []byte          `json:"fingerprint,omitempty"`
	Owner       string          `json:"owner,omitempty"`
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

// ErrFingerprintMismatch is returned by [Keeper.AttemptLockWithOpts] when
// the key is already recorded for a request with another fingerprint: the
// key was reused for a different command. Transports map it to 422
// Unprocessable Content (HTTP) or InvalidArgument (gRPC).
var ErrFingerprintMismatch = errors.New("idempotency: key reused with a different request")

// ErrOwnerMismatch is returned by [Keeper.AttemptLockWithOpts] when the
// caller may not see the key's record: it was recorded by another owner (see
// [WithOwner]), or the [ReplayAuthorizer] refused, its error wrapped.
var ErrOwnerMismatch = errors.New("idempotency: key belongs to another owner")

// AttemptLockOpts customizes a single AttemptLockWithOpts call. All
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

	// Fingerprint identifies the request the key is used for, e.g. a
	// [Fingerprint] of the method and the canonical request body. It is
	// recorded with the key; a later attempt with the same key and another
	// fingerprint fails with [ErrFingerprintMismatch] instead of receiving
	// the stored state. Nil skips the check, as does a record written
	// without a fingerprint.
	Fingerprint []byte
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

	// Release removes a failed request only while lockState still owns the key.
	Release(ctx context.Context, key string, lockState *storages.State) error

	// Delete unconditionally removes a key for administrative use.
	Delete(ctx context.Context, key string) error
}

// Keeper provides duplicate request detection using idempotency keys.
// It supports multiple storage backends for distributed systems.
type Keeper struct {
	storage storages.Storage
	opts    *options
	metrics *keeperMetrics
}

// effectiveKey applies the optional per-context namespace so idempotency keys are
// isolated by tenant/subject. It is applied uniformly to every storage operation
// (AttemptLock/Steal/Complete/Delete). A nil namespace or an empty result leaves
// the key unchanged (backward-compatible default).
func (i *Keeper) effectiveKey(ctx context.Context, key string) string {
	if i.opts.keyNamespace == nil {
		return key
	}
	if ns := i.opts.keyNamespace(ctx); ns != "" {
		return ns + ":" + key
	}
	return key
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

	// Namespace the key so two tenants using the same idempotency key never
	// share a lock or a stored response.
	ek := i.effectiveKey(ctx, key)

	// Embed a per-attempt nonce so two consecutive lock attempts on
	// the same key produce distinct serialized bytes. Storage backends
	// that CAS on byte equality (memory, redis) need this to tell two
	// holders apart. NATS uses revisions and ignores the field.
	owner := i.ownerOf(ctx)
	wire := serializedState{
		Status:      storages.StatusInProgress,
		Nonce:       uuid.NewString(),
		LockedAt:    time.Now(),
		Fingerprint: slices.Clone(opts.Fingerprint),
		Owner:       owner,
	}

	val, err := i.opts.serializer.Serialize(wire)
	if err != nil {
		return false, nil, coreerrs.WrapOperation(err, "serialize state")
	}

	ok, existingVal, lockToken, err := i.storage.AttemptLockWithTTL(ctx, ek, val, opts.LockTTL)
	if err != nil {
		i.metrics.errors.Inc()
		return false, nil, err
	}

	if ok {
		i.metrics.locksAcquired.Inc()
		// Carry the CAS token on the State so Complete can validate
		// our claim later. The State itself isn't serialized at this
		// point; we hand the freshly-built object back to the caller.
		return true, i.heldState(lockToken, wire.Fingerprint, owner), nil
	}

	// Lock failed, key exists. Deserialize existing state via the
	// wire shape and strip the nonce/LockedAt before handing back to
	// the caller — they shouldn't need to know about CAS plumbing.
	var existing serializedState
	if err := i.opts.serializer.Deserialize(existingVal, &existing); err != nil {
		i.metrics.errors.Inc()
		return false, nil, coreerrs.WrapOperation(err, "deserialize existing state")
	}

	// The owner check comes first, so a caller who may not see the record
	// learns nothing about it, not even whether its fingerprint matches.
	if err := i.checkReplay(ctx, existing, owner, opts.Fingerprint); err != nil {
		i.metrics.locksDenied.Inc()
		return false, nil, err
	}

	// Orphan-steal: an InProgress entry whose LockedAt is older than
	// the resolved MaxLockDuration belongs to a crashed (or hung)
	// holder. CAS-steal it so retries don't get stuck behind an
	// abandoned lock until the bucket TTL expires.
	// Only the record's own owner may take it over: a ReplayAuthorizer
	// grants seeing another owner's record, not acting in its place.
	if existing.Status == storages.StatusInProgress && i.mayTakeOver(existing, owner) {
		if maxLock := i.resolveMaxLockDuration(opts); maxLock > 0 &&
			!existing.LockedAt.IsZero() &&
			time.Since(existing.LockedAt) > maxLock {
			// The takeover keeps the record's fingerprint and owner, so a
			// caller without them cannot erase them.
			if len(existing.Fingerprint) > 0 {
				wire.Fingerprint = existing.Fingerprint
			}
			wire.Owner = cmp.Or(existing.Owner, wire.Owner)
			stealVal, err := i.opts.serializer.Serialize(wire)
			if err != nil {
				return false, nil, coreerrs.WrapOperation(err, "serialize state")
			}
			newToken, stealErr := i.storage.Steal(ctx, ek, existingVal, stealVal)
			switch {
			case stealErr == nil:
				i.metrics.locksAcquired.Inc()
				return true, i.heldState(newToken, wire.Fingerprint, wire.Owner), nil
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
	denied := &storages.State{Status: existing.Status, Data: existing.Data}
	denied.SetFingerprint(existing.Fingerprint)
	denied.SetOwner(existing.Owner)
	return false, denied, nil
}

// heldState returns the state of a lock the caller holds: the CAS token, and
// the fingerprint and owner Complete keeps on the record.
func (i *Keeper) heldState(lockToken, fingerprint []byte, owner string) *storages.State {
	st := &storages.State{Status: storages.StatusInProgress}
	st.SetLockToken(lockToken)
	st.SetFingerprint(fingerprint)
	st.SetOwner(owner)
	return st
}

// mayTakeOver reports whether the caller may take over existing's lock: the
// owner check is off, the record has no owner, or it is the caller's.
func (i *Keeper) mayTakeOver(existing serializedState, owner string) bool {
	return i.opts.owner == nil || existing.Owner == "" || existing.Owner == owner
}

// ownerOf returns the caller's owner identity, "" without [WithOwner].
func (i *Keeper) ownerOf(ctx context.Context) string {
	if i.opts.owner == nil {
		return ""
	}
	return i.opts.owner(ctx)
}

// checkReplay decides whether a caller denied the lock may see existing.
// With a [ReplayAuthorizer] it alone decides, for every record ([ErrOwnerMismatch]
// wrapping its refusal); without one, an owner other than the caller's is
// refused ([ErrOwnerMismatch]) and a record without an owner is not checked.
// Then a recorded fingerprint must equal the caller's
// ([ErrFingerprintMismatch]); a record or a caller without one skips that.
func (i *Keeper) checkReplay(ctx context.Context, existing serializedState, owner string, fingerprint []byte) error {
	switch {
	case i.opts.replayAuthorizer != nil:
		if err := i.opts.replayAuthorizer(ctx, existing.Owner); err != nil {
			return fmt.Errorf("%w: %w", ErrOwnerMismatch, err)
		}
	case i.opts.owner != nil && existing.Owner != "" && existing.Owner != owner:
		return ErrOwnerMismatch
	}
	if len(fingerprint) > 0 && len(existing.Fingerprint) > 0 && !bytes.Equal(fingerprint, existing.Fingerprint) {
		return ErrFingerprintMismatch
	}
	return nil
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
// a nil lockState, or one without a lock token (such as the state
// AttemptLock returns when the lock was not acquired), returns
// [ErrMissingLockState]; a stolen lock returns [ErrLockStolen].
func (i *Keeper) Complete(ctx context.Context, key string, data any, lockState *storages.State) error {
	if key == "" {
		return ErrEmptyKey
	}
	// A tokenless state must never reach storage: backends treat a nil
	// token as an unconditional overwrite, which would let a caller denied
	// the lock replace the owner's state.
	if len(lockState.LockToken()) == 0 {
		return ErrMissingLockState
	}

	// The record keeps the fingerprint and owner it was locked with, so a
	// later replay is still checked against them.
	state := serializedState{
		Status:      storages.StatusSuccess,
		Data:        data,
		Fingerprint: lockState.Fingerprint(),
		Owner:       lockState.Owner(),
	}

	val, err := i.opts.serializer.Serialize(state)
	if err != nil {
		return coreerrs.WrapOperation(err, "serialize state")
	}

	if err := i.storage.Complete(ctx, i.effectiveKey(ctx, key), val, lockState.LockToken()); err != nil {
		i.metrics.errors.Inc()
		return err
	}
	i.metrics.completions.Inc()
	return nil
}

// Delete unconditionally removes a key for administrative use. An empty key
// returns [ErrEmptyKey].
func (i *Keeper) Delete(ctx context.Context, key string) error {
	if key == "" {
		return ErrEmptyKey
	}
	if err := i.storage.Delete(ctx, i.effectiveKey(ctx, key)); err != nil {
		i.metrics.errors.Inc()
		return err
	}
	i.metrics.deletions.Inc()
	return nil
}

// Release removes a failed request's lock without affecting a newer owner or
// a completed result. Pass the state returned by AttemptLock.
func (i *Keeper) Release(ctx context.Context, key string, lockState *storages.State) error {
	if key == "" {
		return ErrEmptyKey
	}
	if len(lockState.LockToken()) == 0 {
		return ErrMissingLockState
	}
	if err := i.storage.Release(ctx, i.effectiveKey(ctx, key), lockState.LockToken()); err != nil {
		i.metrics.errors.Inc()
		return err
	}
	i.metrics.deletions.Inc()
	return nil
}
