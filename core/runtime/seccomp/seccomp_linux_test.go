// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux && (amd64 || arm64)

package seccomp

import (
	"testing"

	"golang.org/x/sys/unix"
)

// TestBuildFilter_Length verifies the instruction count matches the
// documented formula: len(dangerousSyscalls) + 6 instructions (arch
// LD, arch JEQ, syscall LD, N JEQs, RET ALLOW, RET DENY, RET KILL).
func TestBuildFilter_Length(t *testing.T) {
	prog := buildFilter()
	want := len(dangerousSyscalls) + 6
	if len(prog) != want {
		t.Errorf("buildFilter length: got %d, want %d", len(prog), want)
	}
}

// TestBuildFilter_ArchPrologue verifies the first two instructions
// load seccomp_data.arch and jump to KILL on mismatch. The arch
// prologue is the most important single feature of the filter: if
// it's wrong, the syscall numbers in the denylist are compared
// against numbers from a different arch, which is silently broken.
func TestBuildFilter_ArchPrologue(t *testing.T) {
	prog := buildFilter()

	// pos 0: LD [arch offset]
	if got, want := prog[0].Code, uint16(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS); got != want {
		t.Errorf("prog[0].Code: got %#x, want %#x", got, want)
	}
	if got, want := prog[0].K, uint32(seccompDataArchOffset); got != want {
		t.Errorf("prog[0].K: got %d, want %d", got, want)
	}

	// pos 1: JEQ expectedArch, jt=0, jf=N+3
	if got, want := prog[1].Code, uint16(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K); got != want {
		t.Errorf("prog[1].Code: got %#x, want %#x", got, want)
	}
	if prog[1].K != expectedArch {
		t.Errorf("prog[1].K: got %#x, want %#x (expectedArch)", prog[1].K, expectedArch)
	}
	if prog[1].Jt != 0 {
		t.Errorf("prog[1].Jt: got %d, want 0", prog[1].Jt)
	}
	if want := uint8(len(dangerousSyscalls) + 3); prog[1].Jf != want {
		t.Errorf("prog[1].Jf: got %d, want %d (N+3)", prog[1].Jf, want)
	}
}

// TestBuildFilter_SyscallNrLoad verifies the instruction that loads
// seccomp_data.nr is at position 2, immediately after the arch
// prologue, and before the JEQ chain.
func TestBuildFilter_SyscallNrLoad(t *testing.T) {
	prog := buildFilter()
	if got, want := prog[2].Code, uint16(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS); got != want {
		t.Errorf("prog[2].Code: got %#x, want %#x", got, want)
	}
	if got, want := prog[2].K, uint32(seccompDataNrOffset); got != want {
		t.Errorf("prog[2].K: got %d, want %d", got, want)
	}
}

// TestBuildFilter_JEQChain verifies each JEQ instruction in the
// denylist chain: instruction at position 2+i+1 (i=0..N-1) compares
// against dangerousSyscalls[i] and jumps to DENY on match. The jt
// offset must be `N - i` so that (2+i+1) + 1 + jt = N+4 = DENY.
func TestBuildFilter_JEQChain(t *testing.T) {
	prog := buildFilter()
	n := len(dangerousSyscalls)

	for i, nr := range dangerousSyscalls {
		pos := 3 + i
		inst := prog[pos]
		if got, want := inst.Code, uint16(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K); got != want {
			t.Errorf("prog[%d].Code: got %#x, want %#x", pos, got, want)
		}
		if inst.K != nr {
			t.Errorf("prog[%d].K: got %#x, want %#x (SYS_i)", pos, inst.K, nr)
		}
		wantJt := uint8(n - i)
		if inst.Jt != wantJt {
			t.Errorf("prog[%d].Jt: got %d, want %d", pos, inst.Jt, wantJt)
		}
		if inst.Jf != 0 {
			t.Errorf("prog[%d].Jf: got %d, want 0", pos, inst.Jf)
		}

		// Sanity check that the computed jt actually lands on DENY.
		target := pos + 1 + int(inst.Jt)
		wantTarget := n + 4
		if target != wantTarget {
			t.Errorf("prog[%d] jt target: got %d, want %d (DENY)", pos, target, wantTarget)
		}
	}
}

// TestBuildFilter_ReturnInstructions verifies the three RET
// instructions at the tail of the program. Order matters: RET ALLOW
// must be the fallthrough target, RET ERRNO(EPERM) must be the DENY
// target, and RET KILL_PROCESS must be the arch-mismatch target.
func TestBuildFilter_ReturnInstructions(t *testing.T) {
	prog := buildFilter()
	n := len(dangerousSyscalls)

	type tc struct {
		name string
		pos  int
		want uint32
	}
	cases := []tc{
		{"RET ALLOW (fallthrough)", n + 3, retAllow},
		{"RET DENY (on match)", n + 4, retDeny},
		{"RET KILL (arch mismatch)", n + 5, retKill},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inst := prog[c.pos]
			if got, want := inst.Code, uint16(unix.BPF_RET|unix.BPF_K); got != want {
				t.Errorf("prog[%d].Code: got %#x, want %#x", c.pos, got, want)
			}
			if inst.K != c.want {
				t.Errorf("prog[%d].K: got %#x, want %#x", c.pos, inst.K, c.want)
			}
		})
	}
}

// TestBuildFilter_ArchMismatchLandsOnKill verifies that when the
// arch JEQ fails, the jf offset actually points at the KILL
// instruction, not at DENY or ALLOW. This is the safety fuse that
// protects against syscall-number confusion on multiarch kernels.
func TestBuildFilter_ArchMismatchLandsOnKill(t *testing.T) {
	prog := buildFilter()
	n := len(dangerousSyscalls)

	// Arch check is at position 1. On mismatch jf = N+3, so the
	// next executed instruction is at 1 + 1 + (N+3) = N+5 = KILL.
	target := 1 + 1 + int(prog[1].Jf)
	wantTarget := n + 5
	if target != wantTarget {
		t.Errorf("arch mismatch target: got %d, want %d (KILL)", target, wantTarget)
	}

	// And KILL at position N+5 must actually be RET KILL_PROCESS.
	if prog[wantTarget].K != retKill {
		t.Errorf("KILL position K: got %#x, want %#x", prog[wantTarget].K, retKill)
	}
}

// TestDangerousSyscalls_ExpectedCount is a cheap sanity check that
// the denylist has the expected size. A future edit that accidentally
// deletes an entry (merge conflict resolution, rebase, typo) would
// silently weaken the filter; this test makes that loud.
func TestDangerousSyscalls_ExpectedCount(t *testing.T) {
	const want = 22
	if got := len(dangerousSyscalls); got != want {
		t.Errorf("dangerousSyscalls count: got %d, want %d — if the change is intentional, update this test and the package README", got, want)
	}
}

// TestDangerousSyscalls_RequiredMembership asserts that every
// security-critical entry that the package's threat model promises to
// block is actually present in the denylist. The count test above
// catches deletions but not substitutions: a refactor that swaps
// SYS_BPF for SYS_OPEN would keep the count at 22 and silently let
// eBPF programs reach the kernel from a "hardened" plugin host.
//
// This test is the canonical contract for what the denylist guarantees.
// Adding an entry here without adding the corresponding entry in
// dangerousSyscalls is a compile-time-detectable mistake (the test
// fails); removing an entry from dangerousSyscalls without removing
// it here is also caught.
//
// Source: dangerousSyscalls categories at seccomp_linux.go:33-72.
func TestDangerousSyscalls_RequiredMembership(t *testing.T) {
	// Every entry the README and threat model commit to blocking.
	// Grouped by category for review diffs.
	required := []struct {
		name string
		nr   uint32
	}{
		// Filesystem manipulation.
		{"SYS_MOUNT", uint32(unix.SYS_MOUNT)},
		{"SYS_UMOUNT2", uint32(unix.SYS_UMOUNT2)},
		{"SYS_PIVOT_ROOT", uint32(unix.SYS_PIVOT_ROOT)},
		{"SYS_CHROOT", uint32(unix.SYS_CHROOT)},
		{"SYS_SWAPON", uint32(unix.SYS_SWAPON)},
		{"SYS_SWAPOFF", uint32(unix.SYS_SWAPOFF)},
		// Kernel module loading.
		{"SYS_INIT_MODULE", uint32(unix.SYS_INIT_MODULE)},
		{"SYS_FINIT_MODULE", uint32(unix.SYS_FINIT_MODULE)},
		{"SYS_DELETE_MODULE", uint32(unix.SYS_DELETE_MODULE)},
		// Kernel reload.
		{"SYS_KEXEC_FILE_LOAD", uint32(unix.SYS_KEXEC_FILE_LOAD)},
		// System control.
		{"SYS_REBOOT", uint32(unix.SYS_REBOOT)},
		// Debugging / memory inspection.
		{"SYS_PTRACE", uint32(unix.SYS_PTRACE)},
		{"SYS_PROCESS_VM_READV", uint32(unix.SYS_PROCESS_VM_READV)},
		{"SYS_PROCESS_VM_WRITEV", uint32(unix.SYS_PROCESS_VM_WRITEV)},
		// Namespace manipulation.
		{"SYS_UNSHARE", uint32(unix.SYS_UNSHARE)},
		{"SYS_SETNS", uint32(unix.SYS_SETNS)},
		// Keyring.
		{"SYS_KEYCTL", uint32(unix.SYS_KEYCTL)},
		{"SYS_ADD_KEY", uint32(unix.SYS_ADD_KEY)},
		{"SYS_REQUEST_KEY", uint32(unix.SYS_REQUEST_KEY)},
		// Exotic escalation vectors.
		{"SYS_USERFAULTFD", uint32(unix.SYS_USERFAULTFD)},
		{"SYS_PERF_EVENT_OPEN", uint32(unix.SYS_PERF_EVENT_OPEN)},
		{"SYS_BPF", uint32(unix.SYS_BPF)},
	}

	// Build a set view of the denylist for O(1) lookups.
	have := make(map[uint32]struct{}, len(dangerousSyscalls))
	for _, nr := range dangerousSyscalls {
		have[nr] = struct{}{}
	}

	for _, req := range required {
		if _, ok := have[req.nr]; !ok {
			t.Errorf("dangerousSyscalls is missing %s (nr=%d) — "+
				"the package's threat model commits to blocking it; "+
				"if removal is intentional, update this test and the README",
				req.name, req.nr)
		}
	}
}
