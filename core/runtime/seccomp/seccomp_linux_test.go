// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux && (amd64 || arm64)

package seccomp

import (
	"testing"

	"github.com/stretchr/testify/require"

	"golang.org/x/sys/unix"
)

// TestBuildFilter_Length verifies the instruction count matches the
// documented formula: len(dangerousSyscalls) + 6 instructions (arch
// LD, arch JEQ, syscall LD, N JEQs, RET ALLOW, RET DENY, RET KILL).
func TestBuildFilter_Length(t *testing.T) {
	prog := buildFilter()
	require.Len(t, prog, len(dangerousSyscalls)+6)
}

// TestBuildFilter_ArchPrologue verifies the first two instructions
// load seccomp_data.arch and jump to KILL on mismatch. The arch
// prologue is the most important single feature of the filter: if
// it's wrong, the syscall numbers in the denylist are compared
// against numbers from a different arch, which is silently broken.
func TestBuildFilter_ArchPrologue(t *testing.T) {
	prog := buildFilter()

	// pos 0: LD [arch offset]
	require.Equal(t, uint16(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS), prog[0].Code, "prog[0].Code")
	require.Equal(t, uint32(seccompDataArchOffset), prog[0].K, "prog[0].K")

	// pos 1: JEQ expectedArch, jt=0, jf=N+3
	require.Equal(t, uint16(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K), prog[1].Code, "prog[1].Code")
	require.Equal(t, expectedArch, prog[1].K, "prog[1].K")
	require.Equal(t, uint8(0), prog[1].Jt, "prog[1].Jt")
	require.Equal(t, uint8(len(dangerousSyscalls)+3), prog[1].Jf, "prog[1].Jf (N+3)")
}

// TestBuildFilter_SyscallNrLoad verifies the instruction that loads
// seccomp_data.nr is at position 2, immediately after the arch
// prologue, and before the JEQ chain.
func TestBuildFilter_SyscallNrLoad(t *testing.T) {
	prog := buildFilter()
	require.Equal(t, uint16(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS), prog[2].Code, "prog[2].Code")
	require.Equal(t, uint32(seccompDataNrOffset), prog[2].K, "prog[2].K")
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
		require.Equal(t, uint16(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K), inst.Code, "prog[%d].Code", pos)
		require.Equal(t, nr, inst.K, "prog[%d].K", pos)
		wantJt := uint8(n - i)
		require.Equal(t, wantJt, inst.Jt, "prog[%d].Jt", pos)
		require.Equal(t, uint8(0), inst.Jf, "prog[%d].Jf", pos)

		// Sanity check that the computed jt actually lands on DENY.
		target := pos + 1 + int(inst.Jt)
		require.Equal(t, n+4, target, "prog[%d] jt target (DENY)", pos)
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
			require.Equal(t, uint16(unix.BPF_RET|unix.BPF_K), inst.Code, "prog[%d].Code", c.pos)
			require.Equal(t, c.want, inst.K, "prog[%d].K", c.pos)
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
	require.Equal(t, wantTarget, target, "arch mismatch target (KILL)")

	// And KILL at position N+5 must actually be RET KILL_PROCESS.
	require.Equal(t, retKill, prog[wantTarget].K, "KILL position K")
}

// TestDangerousSyscalls_ExpectedCount is a cheap sanity check that
// the denylist has the expected size. A future edit that accidentally
// deletes an entry (merge conflict resolution, rebase, typo) would
// silently weaken the filter; this test makes that loud.
func TestDangerousSyscalls_ExpectedCount(t *testing.T) {
	require.Len(t, dangerousSyscalls, 22, "if the change is intentional, update this test and the package README")
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
		_, ok := have[req.nr]
		require.True(t, ok, "dangerousSyscalls is missing %s (nr=%d) — "+
			"the package's threat model commits to blocking it; "+
			"if removal is intentional, update this test and the README",
			req.name, req.nr)
	}
}
