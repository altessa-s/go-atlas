// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storages

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/panics"
)

// Limit is one key of a multi-key admission: the arguments of one
// [Storage.Allow] call.
type Limit struct {
	// Key identifies the window, as the key of [Storage.Allow].
	Key string
	// Limit is the maximum number of requests in the window.
	Limit int64
	// Period is the length of the sliding window.
	Period time.Duration
}

// MultiStorage is a [Storage] that admits a request against several keys
// atomically. It is optional: [AllowAll] uses it when the storage implements
// it and fails with [ErrMultiKeyUnsupported] otherwise.
type MultiStorage interface {
	Storage

	// AllowAll admits one request against every limit, or against none.
	//
	// Either every key admits and a slot is taken on each, or no slot is
	// taken on any key. The returned infos are indexed like limits:
	//
	//   - On admission, infos[i] is the state of limits[i] after the slot
	//     was taken, as [Storage.Allow] reports it.
	//   - On refusal, infos[i] is the state of limits[i] without a slot
	//     taken: Remaining is 0 exactly for the keys that refused, and the
	//     free slots of a key that would have admitted. Reset is the time
	//     the oldest request of the window frees a slot, 0 for an empty
	//     window. The error is a [*KeyError] for the lowest refusing index
	//     that wraps [ErrLimitExceeded].
	//
	// A key named twice fails with a [*KeyError] for the second occurrence
	// that wraps [ErrDuplicateKey], before any state is read: one key has
	// one window, so two limits for it cannot both be applied. An empty
	// list, an empty key or a non-positive limit or period is a programming
	// error and panics, as it does in [Storage.Allow]; see [ValidateLimits].
	AllowAll(ctx context.Context, limits []Limit) ([]LimitInfo, error)
}

var (
	// ErrMultiKeyUnsupported is returned by [AllowAll] for a storage that
	// cannot admit several keys atomically.
	ErrMultiKeyUnsupported = errors.New("storage does not support atomic multi-key admission")

	// ErrDuplicateKey is wrapped by the [*KeyError] of a multi-key call
	// that names one key twice.
	ErrDuplicateKey = errors.New("duplicate key in multi-key admission")
)

// KeyError reports the limit of a multi-key call that failed it. Its message
// names the index only: keys often carry client identity (an IP address, a
// token), so the key is a field for the caller and never part of the text.
type KeyError struct {
	// Index is the position of the failing limit in the call.
	Index int
	// Key is the key of that limit.
	Key string
	// Err is the cause, such as [ErrLimitExceeded] or [ErrDuplicateKey].
	Err error
}

// Error returns the cause prefixed with the index of the failing limit.
func (e *KeyError) Error() string {
	return "limit " + strconv.Itoa(e.Index) + ": " + e.Err.Error()
}

// Unwrap returns the cause.
func (e *KeyError) Unwrap() error { return e.Err }

// AllowAll admits one request against every limit atomically through s; see
// [MultiStorage.AllowAll]. It fails with [ErrMultiKeyUnsupported] when s does
// not implement [MultiStorage]: it never falls back to sequential
// [Storage.Allow] calls, which would take a slot on the keys that admit
// before a later key refuses.
func AllowAll(ctx context.Context, s Storage, limits ...Limit) ([]LimitInfo, error) {
	m, ok := s.(MultiStorage)
	if !ok {
		return nil, ErrMultiKeyUnsupported
	}
	return m.AllowAll(ctx, limits)
}

// smallDuplicateScan is the call size up to which [ValidateLimits] compares
// keys pairwise instead of building a set.
const smallDuplicateScan = 8

// ValidateLimits checks the arguments of [MultiStorage.AllowAll] for an
// implementation. It panics on a programming error, as [Storage.Allow] does:
// an empty list, an empty key, or a non-positive limit or period. It returns
// a [*KeyError] wrapping [ErrDuplicateKey] for the second occurrence of a key
// named twice.
func ValidateLimits(limits []Limit) error {
	panics.Must(len(limits) > 0, "limits must not be empty")
	for _, l := range limits {
		panics.Must(l.Key != "", "key must not be empty")
		panics.Must(l.Limit > 0, "limit must be greater than 0")
		panics.Must(l.Period > 0, "period must be greater than 0")
	}
	if len(limits) <= smallDuplicateScan {
		for i := 1; i < len(limits); i++ {
			for j := range i {
				if limits[i].Key == limits[j].Key {
					return &KeyError{Index: i, Key: limits[i].Key, Err: ErrDuplicateKey}
				}
			}
		}
		return nil
	}
	seen := make(map[string]struct{}, len(limits))
	for i, l := range limits {
		if _, dup := seen[l.Key]; dup {
			return &KeyError{Index: i, Key: l.Key, Err: ErrDuplicateKey}
		}
		seen[l.Key] = struct{}{}
	}
	return nil
}
