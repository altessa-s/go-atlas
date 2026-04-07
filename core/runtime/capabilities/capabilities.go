// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package capabilities

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors for programmatic inspection via [errors.Is]. Errors
// returned from the Linux-specific syscalls also preserve the
// underlying [syscall.Errno] via multi-wrap, so callers can
// additionally use [errors.As] to recover the raw kernel error for
// branching logic.
var (
	// ErrUnsupported is returned when the running platform does not
	// support Linux capabilities (non-Linux platforms).
	ErrUnsupported = errors.New("capabilities: platform does not support Linux capabilities")

	// ErrFailed wraps any error from the underlying capset(2),
	// capget(2), or prctl(2) syscall. The wrap chain preserves the
	// kernel errno so callers can match on [syscall.Errno] via
	// [errors.As] in addition to matching the sentinel via
	// [errors.Is].
	ErrFailed = errors.New("capabilities: capset failed")

	// ErrInvalidOption is returned when an operator supplies a bad
	// capability name or requests an impossible operation (e.g.
	// raising an ambient cap that is not in the permitted set).
	ErrInvalidOption = errors.New("capabilities: invalid option")
)

// Cap is a strongly-typed Linux capability bit. Values mirror the
// kernel's CAP_* numbering; the distinct type prevents accidental
// mixing with plain int arguments.
type Cap int

// Sets is the snapshot of every capability set for the calling thread.
// Returned by [Get]; passed to [Apply] when callers want transactional
// control over the effective/permitted/inheritable masks in one call.
// Every field is a bitmask indexed by [Cap] value: bit N set means
// capability N is present in that set.
type Sets struct {
	// Effective is the bits currently in force for privileged syscalls.
	Effective uint64

	// Permitted is the ceiling from which Effective can be raised.
	Permitted uint64

	// Inheritable is passed to children across execve.
	Inheritable uint64

	// Bounding is the ceiling from which Permitted can be raised. A
	// bit dropped from Bounding can never be re-acquired by the
	// calling thread or any of its descendants.
	Bounding uint64

	// Ambient is preserved across a non-privileged execve when also
	// present in Permitted and Inheritable.
	Ambient uint64
}

// Get returns the current capability snapshot for the calling thread.
// On non-Linux platforms Get returns (Sets{}, [ErrUnsupported]).
func Get() (Sets, error) {
	return getSnapshot()
}

// DropAll drops every capability from every set for the calling
// thread, including the bounding and ambient sets. After DropAll
// returns nil the process has zero capabilities and can never raise
// any of them again for the lifetime of the process.
func DropAll() error {
	return dropAllExcept(nil)
}

// DropAllExcept drops every capability except those in keep. The
// listed capabilities are preserved in the Effective, Permitted, and
// Bounding sets; Inheritable and Ambient are cleared (standard
// practice for "drop everything except what I need at startup"). An
// empty keep list is equivalent to [DropAll].
//
// Duplicate entries in keep are harmless and silently deduplicated.
func DropAllExcept(keep ...Cap) error {
	return dropAllExcept(keep)
}

// Apply installs s on the calling thread atomically where the kernel
// permits and via a best-effort sequence otherwise. Apply validates
// that no Effective/Permitted/Inheritable bit exceeds the current
// Bounding set — the kernel rejects that combination with EPERM, and
// failing early produces a clearer error.
//
// Apply does not raise Bounding or Ambient bits; it can only lower
// them. Use [AmbientRaise] for additive ambient changes.
func Apply(s Sets) error {
	return applySnapshot(s)
}

// BoundingDrop removes c from the bounding set. This is irreversible
// for an unprivileged process: once a bit is outside the bounding set
// no descendant can ever acquire it. Calling BoundingDrop on a bit
// already absent from the bounding set is a no-op and succeeds.
func BoundingDrop(c Cap) error {
	return boundingDrop(c)
}

// AmbientClearAll clears every ambient capability bit. Equivalent to
// prctl(PR_CAP_AMBIENT, PR_CAP_AMBIENT_CLEAR_ALL).
func AmbientClearAll() error {
	return ambientClearAll()
}

// AmbientRaise adds c to the ambient set. Fails when c is not already
// present in both the permitted and inheritable sets — that is a
// kernel invariant, not a package choice.
func AmbientRaise(c Cap) error {
	return ambientRaise(c)
}

// AmbientLower removes c from the ambient set. Succeeds whether or
// not c was previously present.
func AmbientLower(c Cap) error {
	return ambientLower(c)
}

// ParseName resolves a canonical "CAP_*" string to its [Cap] value.
// Matching is case-insensitive and tolerates leading/trailing
// whitespace. Unknown names return a wrapped [ErrInvalidOption].
//
// Intended for operators writing YAML configuration. Callers that
// know the capability at compile time should use the typed constants
// (e.g. [CAP_NET_BIND_SERVICE]) instead.
func ParseName(name string) (Cap, error) {
	key := strings.ToUpper(strings.TrimSpace(name))
	if c, ok := nameToCap[key]; ok {
		return c, nil
	}
	return 0, fmt.Errorf("%w: unknown capability %q", ErrInvalidOption, name)
}

// String returns the canonical "CAP_*" name for c, or a synthetic
// "CAP_UNKNOWN(N)" form for out-of-range values. String is safe to
// call on the zero value.
func (c Cap) String() string {
	if name, ok := capToName[c]; ok {
		return name
	}
	return fmt.Sprintf("CAP_UNKNOWN(%d)", int(c))
}
