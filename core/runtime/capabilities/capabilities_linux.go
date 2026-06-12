// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux

package capabilities

import (
	"fmt"
	"runtime"

	"golang.org/x/sys/unix"
)

// capHeader returns a [unix.CapUserHeader] requesting version 3 (64-bit
// masks, kernel 2.6.26+). The Pid field is 0, meaning "the calling
// thread", which is the only safe target for unprivileged processes.
func capHeader() *unix.CapUserHeader {
	return &unix.CapUserHeader{
		Version: unix.LINUX_CAPABILITY_VERSION_3,
		Pid:     0,
	}
}

const (
	// capDataLen is the length of the [unix.CapUserData] array required
	// by capability version 3: two 32-bit words per set (low + high).
	capDataLen = 2

	// halfWord is the bit width of one capability data word. The kernel
	// stores each mask as two uint32 halves; shifting by halfWord moves
	// between the low and high words.
	halfWord = 32
)

// has reports whether c is set in mask.
func has(mask uint64, c Cap) bool {
	return mask&(uint64(1)<<uint(c)) != 0
}

// setBit returns mask with c added.
func setBit(mask uint64, c Cap) uint64 {
	return mask | (uint64(1) << uint(c))
}

// toData packs effective/permitted/inheritable masks into the kernel's
// two-word representation.
func toData(eff, perm, inh uint64) [capDataLen]unix.CapUserData {
	var d [capDataLen]unix.CapUserData
	d[0].Effective = uint32(eff)
	d[1].Effective = uint32(eff >> halfWord)
	d[0].Permitted = uint32(perm)
	d[1].Permitted = uint32(perm >> halfWord)
	d[0].Inheritable = uint32(inh)
	d[1].Inheritable = uint32(inh >> halfWord)
	return d
}

// fromData unpacks the kernel's two-word representation into uint64
// masks.
func fromData(d [capDataLen]unix.CapUserData) (eff, perm, inh uint64) {
	eff = uint64(d[0].Effective) | uint64(d[1].Effective)<<halfWord
	perm = uint64(d[0].Permitted) | uint64(d[1].Permitted)<<halfWord
	inh = uint64(d[0].Inheritable) | uint64(d[1].Inheritable)<<halfWord
	return eff, perm, inh
}

// capget calls capget(2) for the calling thread and returns the three
// thread-local masks. The bounding and ambient sets are read
// separately via prctl — see [readBounding] and [readAmbient].
func capget() (eff, perm, inh uint64, err error) {
	hdr := capHeader()
	var data [capDataLen]unix.CapUserData
	if e := unix.Capget(hdr, &data[0]); e != nil {
		return 0, 0, 0, fmt.Errorf("%w: capget: %w", ErrFailed, e)
	}
	eff, perm, inh = fromData(data)
	return eff, perm, inh, nil
}

// capset calls capset(2) for the calling thread, installing the given
// three thread-local masks.
func capset(eff, perm, inh uint64) error {
	hdr := capHeader()
	data := toData(eff, perm, inh)
	if err := unix.Capset(hdr, &data[0]); err != nil {
		return fmt.Errorf("%w: capset: %w", ErrFailed, err)
	}
	return nil
}

// readBounding probes the bounding set one cap at a time via
// PR_CAPBSET_READ. This is the canonical way to read the bounding set
// — there is no syscall that returns it as a mask.
func readBounding() (uint64, error) {
	var mask uint64
	for c := Cap(0); c <= capLastCap; c++ {
		v, err := unix.PrctlRetInt(unix.PR_CAPBSET_READ, uintptr(c), 0, 0, 0)
		if err != nil {
			return 0, fmt.Errorf("%w: prctl(PR_CAPBSET_READ, %s): %w", ErrFailed, c, err)
		}
		if v == 1 {
			mask = setBit(mask, c)
		}
	}
	return mask, nil
}

// readAmbient probes the ambient set one cap at a time via
// PR_CAP_AMBIENT_IS_SET. Same rationale as readBounding.
func readAmbient() (uint64, error) {
	var mask uint64
	for c := Cap(0); c <= capLastCap; c++ {
		v, err := unix.PrctlRetInt(
			unix.PR_CAP_AMBIENT,
			unix.PR_CAP_AMBIENT_IS_SET,
			uintptr(c), 0, 0,
		)
		if err != nil {
			return 0, fmt.Errorf("%w: prctl(PR_CAP_AMBIENT_IS_SET, %s): %w", ErrFailed, c, err)
		}
		if v == 1 {
			mask = setBit(mask, c)
		}
	}
	return mask, nil
}

// getSnapshot is the Linux implementation of [Get]. It reads all five
// sets on the calling thread. Pinning to the OS thread is not required
// for a read-only call — capget targets the thread but does not
// mutate any global state.
func getSnapshot() (Sets, error) {
	eff, perm, inh, err := capget()
	if err != nil {
		return Sets{}, err
	}
	bnd, err := readBounding()
	if err != nil {
		return Sets{}, err
	}
	amb, err := readAmbient()
	if err != nil {
		return Sets{}, err
	}
	return Sets{
		Effective:   eff,
		Permitted:   perm,
		Inheritable: inh,
		Bounding:    bnd,
		Ambient:     amb,
	}, nil
}

// applySnapshot is the Linux implementation of [Apply]. It:
//
//  1. Pins the goroutine to its OS thread for the duration.
//  2. Validates that no requested effective/permitted/inheritable bit
//     exceeds the current bounding set (kernel would reject with
//     EPERM; failing early gives a clearer error).
//  3. Clears ambient caps that should no longer be present. Ambient
//     can only be lowered relative to the desired state in one call;
//     raising happens bit-by-bit after the capset succeeds so the
//     kernel invariant (cap must be in permitted ∩ inheritable) is
//     guaranteed.
//  4. Calls capset(2) with the new effective/permitted/inheritable.
//  5. Drops every bounding-set bit not present in s.Bounding.
//  6. Raises any ambient bits present in s.Ambient that are not
//     currently set.
//
// Steps are ordered so that an abort after any one leaves the thread
// in a state that is "at most as privileged" as before — never more.
//
// On a step-3-through-6 failure the returned error is annotated with
// which step failed and how far the partial application got. This
// matters because the calling thread is left in an intermediate
// state: e.g. capset has installed the new effective/permitted but
// some bounding-drop syscalls have not yet run, so the caller's
// "actual" capabilities diverge from the requested snapshot. Without
// the step annotation an operator sees only the underlying errno
// and has no way to reconstruct what state the thread is in.
//
// When a failure occurs after the thread state has already been
// mutated (any successful ambient/capset/bounding syscall), the OS
// thread pin is retained — the goroutine carries the lock away on
// exit. Releasing a thread whose capability state diverged mid-way
// would hand the scheduler a thread that behaves differently from
// its peers. Failures before the first mutation (snapshot read,
// validation) release the pin as usual. Same policy as the sibling
// seccomp package.
func applySnapshot(s Sets) (retErr error) {
	runtime.LockOSThread()
	mutated := false
	defer func() {
		if retErr == nil || !mutated {
			runtime.UnlockOSThread()
		}
	}()

	cur, err := getSnapshot()
	if err != nil {
		return fmt.Errorf("applySnapshot: read current state: %w", err)
	}

	// Validate against the current bounding ceiling. The kernel will
	// reject any effective/permitted/inheritable bit that is not in
	// the bounding set; checking here produces a clearer error than
	// EPERM with no context.
	if s.Effective&^cur.Bounding != 0 {
		return fmt.Errorf(
			"%w: effective mask %#x exceeds bounding %#x",
			ErrInvalidOption, s.Effective, cur.Bounding,
		)
	}
	if s.Permitted&^cur.Bounding != 0 {
		return fmt.Errorf(
			"%w: permitted mask %#x exceeds bounding %#x",
			ErrInvalidOption, s.Permitted, cur.Bounding,
		)
	}
	if s.Inheritable&^cur.Bounding != 0 {
		return fmt.Errorf(
			"%w: inheritable mask %#x exceeds bounding %#x",
			ErrInvalidOption, s.Inheritable, cur.Bounding,
		)
	}

	// Lower ambient bits first. This is safe regardless of whether
	// the upcoming capset succeeds, because lowering can only reduce
	// privilege. A failure here means no other syscall has run yet,
	// so the thread state matches the pre-call snapshot minus any
	// successfully-lowered ambient bits.
	toLower := cur.Ambient &^ s.Ambient
	for c := Cap(0); c <= capLastCap; c++ {
		if !has(toLower, c) {
			continue
		}
		if err := ambientLower(c); err != nil {
			return fmt.Errorf(
				"applySnapshot: step 3 (ambient lower of %s) failed; "+
					"thread state: ambient bits up to but not including %s have been lowered, "+
					"capset/bounding/ambient-raise have NOT run: %w",
				c, c, err,
			)
		}
		mutated = true
	}

	// Install the new effective/permitted/inheritable. If this fails
	// the capset itself was rejected, so the thread keeps its
	// pre-call effective/permitted/inheritable but with the lowered
	// ambient state from step 3.
	if err := capset(s.Effective, s.Permitted, s.Inheritable); err != nil {
		return fmt.Errorf(
			"applySnapshot: step 4 (capset eff=%#x perm=%#x inh=%#x) failed; "+
				"thread state: ambient lowered, capset rejected, "+
				"effective/permitted/inheritable still match the pre-call snapshot: %w",
			s.Effective, s.Permitted, s.Inheritable, err,
		)
	}
	mutated = true

	// Drop bounding bits that should no longer be present. This is
	// irreversible — once dropped from bounding, the bit cannot be
	// re-acquired by this thread or any descendant. A failure here
	// is the most dangerous case because capset has already
	// committed: the requested effective/permitted/inheritable are
	// installed, but the bounding ceiling is partially old.
	toDrop := cur.Bounding &^ s.Bounding
	for c := Cap(0); c <= capLastCap; c++ {
		if !has(toDrop, c) {
			continue
		}
		if err := boundingDrop(c); err != nil {
			return fmt.Errorf(
				"applySnapshot: step 5 (bounding drop of %s) failed; "+
					"thread state: capset committed but bounding ceiling is "+
					"partially the old set — bits up to but not including %s have been dropped, "+
					"every bit at %s and above is still in the bounding set: %w",
				c, c, c, err,
			)
		}
	}

	// Raise ambient bits that should be present but are not. The
	// kernel enforces that cap is in permitted ∩ inheritable, which
	// is now true because capset has already succeeded with the new
	// masks. A failure here means capset and bounding-drops are
	// fully committed but ambient is partial.
	toRaise := s.Ambient &^ cur.Ambient
	for c := Cap(0); c <= capLastCap; c++ {
		if !has(toRaise, c) {
			continue
		}
		if err := ambientRaise(c); err != nil {
			return fmt.Errorf(
				"applySnapshot: step 6 (ambient raise of %s) failed; "+
					"thread state: capset and bounding fully committed, "+
					"ambient bits up to but not including %s have been raised: %w",
				c, c, err,
			)
		}
	}
	return nil
}

// dropAllExcept is the Linux implementation of [DropAll] /
// [DropAllExcept]. It builds a Sets snapshot with the requested caps
// preserved in effective/permitted/bounding and nothing in
// inheritable/ambient, then delegates to [applySnapshot].
func dropAllExcept(keep []Cap) error {
	var mask uint64
	for _, c := range keep {
		if c < 0 || c > capLastCap {
			return fmt.Errorf("%w: %s is out of range", ErrInvalidOption, c)
		}
		mask = setBit(mask, c)
	}
	return applySnapshot(Sets{
		Effective:   mask,
		Permitted:   mask,
		Inheritable: 0,
		Bounding:    mask,
		Ambient:     0,
	})
}

// boundingDrop is the Linux implementation of [BoundingDrop]. Calling
// PR_CAPBSET_DROP on a cap not currently in the bounding set is a
// no-op and returns success, so the caller does not need to check
// first.
func boundingDrop(c Cap) error {
	if c < 0 || c > capLastCap {
		return fmt.Errorf("%w: %s is out of range", ErrInvalidOption, c)
	}
	if err := unix.Prctl(unix.PR_CAPBSET_DROP, uintptr(c), 0, 0, 0); err != nil {
		return fmt.Errorf("%w: prctl(PR_CAPBSET_DROP, %s): %w", ErrFailed, c, err)
	}
	return nil
}

// ambientClearAll is the Linux implementation of [AmbientClearAll].
func ambientClearAll() error {
	if err := unix.Prctl(
		unix.PR_CAP_AMBIENT,
		unix.PR_CAP_AMBIENT_CLEAR_ALL,
		0, 0, 0,
	); err != nil {
		return fmt.Errorf("%w: prctl(PR_CAP_AMBIENT_CLEAR_ALL): %w", ErrFailed, err)
	}
	return nil
}

// ambientRaise is the Linux implementation of [AmbientRaise].
func ambientRaise(c Cap) error {
	if c < 0 || c > capLastCap {
		return fmt.Errorf("%w: %s is out of range", ErrInvalidOption, c)
	}
	if err := unix.Prctl(
		unix.PR_CAP_AMBIENT,
		unix.PR_CAP_AMBIENT_RAISE,
		uintptr(c), 0, 0,
	); err != nil {
		return fmt.Errorf("%w: prctl(PR_CAP_AMBIENT_RAISE, %s): %w", ErrFailed, c, err)
	}
	return nil
}

// ambientLower is the Linux implementation of [AmbientLower].
func ambientLower(c Cap) error {
	if c < 0 || c > capLastCap {
		return fmt.Errorf("%w: %s is out of range", ErrInvalidOption, c)
	}
	if err := unix.Prctl(
		unix.PR_CAP_AMBIENT,
		unix.PR_CAP_AMBIENT_LOWER,
		uintptr(c), 0, 0,
	); err != nil {
		return fmt.Errorf("%w: prctl(PR_CAP_AMBIENT_LOWER, %s): %w", ErrFailed, c, err)
	}
	return nil
}
