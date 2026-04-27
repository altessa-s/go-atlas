// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storages

import "errors"

// ErrEmptyKey is returned by [Storage] implementations when an empty
// idempotency key is supplied. An empty key would silently disable dedupe
// — every "lock" would succeed without writing anything — so callers must
// validate keys before reaching the storage layer.
var ErrEmptyKey = errors.New("idempotency: empty key")

// ErrLockStolen is returned by [Storage.Complete] when the lock has
// been taken over by another holder between [Storage.AttemptLock] and
// the [Storage.Complete] call. Typically caused by the lock TTL
// expiring while the original holder was still processing, allowing
// another caller's AttemptLock to succeed. The current operation
// must abort instead of overwriting the result of the new holder.
var ErrLockStolen = errors.New("idempotency: lock stolen by another holder")

// ErrMissingLockState is returned by [Idempotency.Complete] when the
// caller passes a nil lock state. Complete needs the *State returned
// by AttemptLock to validate the CAS token; passing nil would silently
// disable the stolen-lock guard.
var ErrMissingLockState = errors.New("idempotency: missing lock state")
