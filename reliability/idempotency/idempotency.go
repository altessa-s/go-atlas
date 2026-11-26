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
	"uuid"

	"github.com/altessa-s/go-atlas-core/encoding/serializer"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// serializedState is the on-the-wire shape Keeper writes through the
// underlying Storage. The Nonce field is added on every AttemptLock to
// guarantee that two consecutive in-progress writes for the same key
// produce distinct bytes — required for byte-equality CAS guards in
// the memory and redis backends. NATS uses revisions and ignores the
// nonce. LockIssuer and LockTick let the same Keeper recognize a fresh
// lock using elapsed monotonic time. [State] omits these fields.
type serializedState struct {
	Status      Status `json:"status"`
	Data        any    `json:"data,omitempty"`
	Nonce       string `json:"nonce,omitempty"`
	LockIssuer  string `json:"lock_issuer,omitempty"`
	LockTick    int64  `json:"lock_tick,omitzero"`
	Fingerprint []byte `json:"fingerprint,omitempty"`
	Owner       string `json:"owner,omitempty"`
}

// ErrEmptyKey is returned by Keeper operations when the idempotency key is empty.
// It shares its identity with [storages.ErrEmptyKey].
var ErrEmptyKey = storages.ErrEmptyKey

// ErrMissingLockState is returned by [Keeper.Complete] and [Keeper.Release]
// when the supplied state has no ownership token, including a nil state.
// It shares its identity with [storages.ErrMissingLockState].
var ErrMissingLockState = storages.ErrMissingLockState

// ErrLockStolen is returned by [Keeper.Complete] and [Keeper.Release] when
// the caller no longer owns the lock. It shares its identity with [storages.ErrLockStolen].
var ErrLockStolen = storages.ErrLockStolen

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
	// InProgress entry older than this, it asks an age-aware backend
	// to take ownership. For lease-based stores such as NATS, this
	// caps the lifetime of a lock acquired by this call.
	// Pass <= 0 to use the Keeper default
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
	AttemptLock(ctx context.Context, key string) (bool, *State, error)

	// AttemptLockWithOpts is like [Idempotency.AttemptLock] but
	// applies the per-call overrides in opts. Use this when a
	// particular key needs a different lock lifetime (e.g. short-lived
	// OTP tokens vs long-running webhook processing) or a different
	// orphan-reclaim threshold.
	AttemptLockWithOpts(ctx context.Context, key string, opts AttemptLockOpts) (bool, *State, error)

	// Complete marks the key as successfully processed. The lockState
	// must be the *State returned by [Idempotency.AttemptLock] —
	// its embedded CAS token gates the write. Returns
	// [ErrLockStolen] when the lock expired or was replaced after
	// AttemptLock, [ErrMissingLockState] when lockState
	// is nil.
	Complete(ctx context.Context, key string, data any, lockState *State) error

	// Release removes a failed request only while lockState still owns the key.
	Release(ctx context.Context, key string, lockState *State) error

	// Delete unconditionally removes a key for administrative use.
	Delete(ctx context.Context, key string) error
}

// Keeper provides duplicate request detection using idempotency keys.
// It supports multiple storage backends for distributed systems.
type Keeper struct {
	storage    storages.Storage
	opts       *options
	metrics    *keeperMetrics
	lockIssuer string
	lockEpoch  time.Time
	lockLeases bool
}

// effectiveKey returns the storage key for key: the tuple ("namespace",
// namespace, key) composed by [ComposeKey], the namespace being the
// per-context one (typically tenant/subject). It is applied uniformly to every
// storage operation (AttemptLock/Steal/Complete/Release/Delete). The key is
// wrapped even when the namespace is empty — a nil [KeyNamespaceFunc] and an
// empty result are the same empty namespace — so no caller's raw key can
// reach another namespace's storage key.
func (i *Keeper) effectiveKey(ctx context.Context, key string) string {
	var ns string
	if i.opts.keyNamespace != nil {
		ns = i.opts.keyNamespace(ctx)
	}
	return ComposeKey("namespace", ns, key)
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

	leaseStore, ok := storage.(storages.LockLeaseStorage)
	return &Keeper{
		storage:    storage,
		opts:       options,
		metrics:    newKeeperMetrics(options.meter),
		lockIssuer: uuid.New().String(),
		lockEpoch:  time.Now(),
		lockLeases: ok && leaseStore.UsesLockLease(),
	}
}

// AttemptLock tries to acquire a lock for the given key using the
// backend's configured TTL and the Keeper's configured
// MaxLockDuration. An empty key returns [ErrEmptyKey].
func (i *Keeper) AttemptLock(ctx context.Context, key string) (bool, *State, error) {
	return i.AttemptLockWithOpts(ctx, key, AttemptLockOpts{})
}

// AttemptLockWithOpts is like [Keeper.AttemptLock] but applies the
// per-call overrides in opts. See [AttemptLockOpts] for field
// semantics.
//
// On collision with an in-progress entry, AttemptLockWithOpts can ask the
// store to reclaim locks older than MaxLockDuration. A successful takeover
// returns a state with a fresh lock token. Lease-based stores reclaim
// expired locks through ordinary acquisition.
func (i *Keeper) AttemptLockWithOpts(ctx context.Context, key string, opts AttemptLockOpts) (bool, *State, error) {
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
		Status:      StatusInProgress,
		Nonce:       uuid.New().String(),
		Fingerprint: slices.Clone(opts.Fingerprint),
		Owner:       owner,
	}

	maxAge := i.resolveMaxLockDuration(opts)
	lockTTL := i.acquisitionTTL(opts.LockTTL, maxAge)
	i.stampLock(&wire)
	val, err := i.opts.serializer.Serialize(wire)
	if err != nil {
		return false, nil, coreerrs.WrapOperation(err, "serialize state")
	}

	var existingVal []byte
	const lockAttempts = 2
	for range lockAttempts {
		ok, current, lockToken, lockErr := i.storage.AttemptLockWithTTL(ctx, ek, val, lockTTL)
		if lockErr != nil {
			i.metrics.errors.Inc()
			return false, nil, lockErr
		}
		if ok {
			i.metrics.locksAcquired.Inc()
			return true, i.heldState(lockToken, wire.Fingerprint, owner), nil
		}
		if len(current) != 0 {
			existingVal = current
			break
		}
	}
	if len(existingVal) == 0 {
		i.metrics.locksDenied.Inc()
		return false, &State{Status: StatusInProgress}, nil
	}

	existing, err := i.readCollision(ctx, existingVal, owner, opts.Fingerprint)
	if err != nil {
		return false, nil, err
	}

	// A ReplayAuthorizer grants visibility, not ownership. Only the
	// record's owner can replace its lock.
	if !i.lockLeases && i.canReclaim(existing, owner, maxAge) {
		if len(existing.Fingerprint) > 0 {
			wire.Fingerprint = existing.Fingerprint
		}
		wire.Owner = cmp.Or(existing.Owner, owner)
		i.stampLock(&wire)
		replacement, err := i.opts.serializer.Serialize(wire)
		if err != nil {
			return false, nil, coreerrs.WrapOperation(err, "serialize state")
		}
		stolen, current, token, err := i.storage.Steal(ctx, ek, existingVal, replacement, maxAge, opts.LockTTL)
		if err != nil {
			i.metrics.errors.Inc()
			return false, nil, err
		}
		if stolen {
			i.metrics.locksAcquired.Inc()
			return true, i.heldState(token, wire.Fingerprint, wire.Owner), nil
		}
		switch {
		case len(current) == 0:
			existing = serializedState{Status: StatusInProgress}
		case !bytes.Equal(current, existingVal):
			// Concurrent completion or takeover may have changed the
			// replay permissions or fingerprint along with the payload.
			existing, err = i.readCollision(ctx, current, owner, opts.Fingerprint)
			if err != nil {
				return false, nil, err
			}
		}
	}

	i.metrics.locksDenied.Inc()
	denied := &State{Status: existing.Status, Data: existing.Data}
	denied.SetFingerprint(existing.Fingerprint)
	denied.SetOwner(existing.Owner)
	return false, denied, nil
}

// heldState returns the state of a lock the caller holds: the CAS token, and
// the fingerprint and owner Complete keeps on the record.
func (i *Keeper) heldState(lockToken, fingerprint []byte, owner string) *State {
	st := &State{Status: StatusInProgress}
	st.SetLockToken(lockToken)
	st.SetFingerprint(fingerprint)
	st.SetOwner(owner)
	return st
}

func (i *Keeper) readCollision(ctx context.Context, val []byte, owner string, fingerprint []byte) (serializedState, error) {
	var existing serializedState
	if err := i.opts.serializer.Deserialize(val, &existing); err != nil {
		i.metrics.errors.Inc()
		return serializedState{}, coreerrs.WrapOperation(err, "deserialize existing state")
	}
	if err := i.checkReplay(ctx, existing, owner, fingerprint); err != nil {
		i.metrics.locksDenied.Inc()
		return serializedState{}, err
	}
	return existing, nil
}

func (i *Keeper) acquisitionTTL(requested, maxAge time.Duration) time.Duration {
	if i.lockLeases && maxAge > 0 && (requested <= 0 || maxAge < requested) {
		return maxAge
	}
	return requested
}

func (i *Keeper) stampLock(wire *serializedState) {
	if i.lockLeases {
		return
	}
	wire.LockIssuer = i.lockIssuer
	wire.LockTick = time.Since(i.lockEpoch).Nanoseconds()
}

func (i *Keeper) canReclaim(existing serializedState, owner string, maxAge time.Duration) bool {
	if maxAge <= 0 || existing.Status != StatusInProgress || (existing.Owner != "" && existing.Owner != owner) {
		return false
	}
	if existing.LockIssuer == i.lockIssuer {
		// The stamp precedes acquisition, so a fresh local stamp lets us
		// skip the write attempt. The backend still decides actual age.
		now := time.Since(i.lockEpoch).Nanoseconds()
		if existing.LockTick >= 0 && existing.LockTick <= now && now-existing.LockTick < maxAge.Nanoseconds() {
			return false
		}
	}
	return true
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
	case existing.Owner != "" && (i.opts.owner == nil || existing.Owner != owner):
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
func (i *Keeper) Complete(ctx context.Context, key string, data any, lockState *State) error {
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
		Status:      StatusSuccess,
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
func (i *Keeper) Release(ctx context.Context, key string, lockState *State) error {
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
