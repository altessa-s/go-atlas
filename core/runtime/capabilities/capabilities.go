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
// THREAD, including the bounding and ambient sets. After DropAll
// returns nil the calling thread has zero capabilities and can never
// raise any of them again for the lifetime of the process.
//
// Per-thread scope (read this before using):
//
// capset(2) is per-thread and Linux does not expose a way to apply a
// new capability set to every thread of a process from userspace. The
// Go runtime has typically already created several OS threads (sysmon,
// GC, netpoll, GOMAXPROCS workers) by the time user code runs, so a
// post-startup DropAll only restricts the goroutine's current thread
// — peer threads keep their original capabilities, and any goroutine
// the scheduler later places on a peer thread can still call
// privileged syscalls.
//
// For a real process-wide drop, drop capabilities externally before
// the Go binary starts: a systemd unit with CapabilityBoundingSet=,
// a container runtime with --cap-drop, or a small C launcher that
// drops privileges before execve(2). This package then becomes a
// inspection / defense-in-depth helper rather than the primary
// mechanism. See [github.com/altessa-s/go-atlas/core/plugins].
func DropAll() error {
	return dropAllExcept(nil)
}

// DropAllExcept drops every capability except those in keep on the
// calling THREAD. The listed capabilities are preserved in the
// Effective, Permitted, and Bounding sets; Inheritable and Ambient
// are cleared (standard practice for "drop everything except what I
// need at startup"). An empty keep list is equivalent to [DropAll].
//
// Per-thread scope: see the [DropAll] doc comment for the Go-runtime
// caveat. DropAllExcept inherits the same limitation.
//
// Duplicate entries in keep are harmless and silently deduplicated.
func DropAllExcept(keep ...Cap) error {
	return dropAllExcept(keep)
}

// Apply installs s on the calling THREAD via the standard
// capset(2) / PR_CAPBSET_DROP / PR_CAP_AMBIENT_* syscall sequence.
// The sequence is multi-step (lower ambient, then capset for
// effective/permitted/inheritable, then drop bounding bits, then
// raise ambient bits) and ordered so that an abort after any one step
// leaves the thread "at most as privileged as before" — never more.
// Apply is NOT atomic; the kernel exposes no syscall to install all
// five sets in one call. Apply validates that no
// Effective/Permitted/Inheritable bit exceeds the current Bounding
// set up-front, since the kernel would otherwise reject that
// combination with EPERM mid-sequence and produce a confusing error.
//
// Apply does not raise Bounding or Ambient bits; it can only lower
// them. Use [AmbientRaise] for additive ambient changes.
//
// Per-thread scope: see the [DropAll] doc comment for the Go-runtime
// caveat. Apply inherits the same limitation.
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
	if c, ok := nameToCap.Get(key); ok {
		return c, nil
	}
	return 0, fmt.Errorf("%w: unknown capability %q", ErrInvalidOption, name)
}

// String returns the canonical "CAP_*" name for c, or a synthetic
// "CAP_UNKNOWN(N)" form for out-of-range values. String never panics
// for any int value of Cap. Note: the zero value Cap(0) corresponds to
// CAP_CHOWN, not to a sentinel "no capability" — there is no such
// sentinel in this package; use a separate boolean or [Sets] mask if
// you need "absent" semantics.
func (c Cap) String() string {
	if name, ok := capToName.Get(c); ok {
		return name
	}
	return fmt.Sprintf("CAP_UNKNOWN(%d)", int(c))
}

// Has reports whether c is set in any of the Sets masks named in
// `kinds`. With no kinds (the zero-arg form) Has reports whether c is
// in the Effective set, since that is the mask the kernel checks at
// syscall time. Pass one or more [Effective], [Permitted],
// [Inheritable], [Bounding], or [Ambient] kinds to query other masks.
//
// Direct bit-twiddling on the Sets fields (e.g. `s.Effective & (1 << c)`)
// is also supported and is the only option in cgo-free packages that
// vendor [Sets] without importing this package — Has is provided as
// the more readable form for new code.
func (s Sets) Has(c Cap, kinds ...SetKind) bool {
	if c < 0 || c >= 64 {
		return false
	}
	bit := uint64(1) << uint(c)
	if len(kinds) == 0 {
		return s.Effective&bit != 0
	}
	for _, k := range kinds {
		var mask uint64
		switch k {
		case Effective:
			mask = s.Effective
		case Permitted:
			mask = s.Permitted
		case Inheritable:
			mask = s.Inheritable
		case Bounding:
			mask = s.Bounding
		case Ambient:
			mask = s.Ambient
		default:
			continue
		}
		if mask&bit != 0 {
			return true
		}
	}
	return false
}

// With returns a copy of s with c added to each of the named masks.
// With no kinds the call is a no-op (returns s unchanged) — pass one
// or more [SetKind] values to indicate which masks to mutate. The
// receiver itself is not modified.
//
// With does not enforce kernel invariants between masks (e.g. Effective
// must be a subset of Permitted): callers building a snapshot for
// [Apply] are responsible for the consistency. Use With to compose a
// snapshot, then call Apply, which validates the result against the
// current bounding ceiling and surfaces any kernel-side rejection.
func (s Sets) With(c Cap, kinds ...SetKind) Sets {
	if c < 0 || c >= 64 {
		return s
	}
	bit := uint64(1) << uint(c)
	for _, k := range kinds {
		switch k {
		case Effective:
			s.Effective |= bit
		case Permitted:
			s.Permitted |= bit
		case Inheritable:
			s.Inheritable |= bit
		case Bounding:
			s.Bounding |= bit
		case Ambient:
			s.Ambient |= bit
		}
	}
	return s
}

// SetKind names one of the five capability sets in [Sets]. Used as a
// vararg by [Sets.Has] and [Sets.With] to identify which mask to
// query or mutate.
type SetKind int

// Capability set kinds. Values are package-private integers; the only
// stable identity is the constant name.
const (
	Effective SetKind = iota + 1
	Permitted
	Inheritable
	Bounding
	Ambient
)
