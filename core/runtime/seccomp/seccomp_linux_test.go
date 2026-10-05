// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux && (amd64 || arm64)

package seccomp

import (
	"encoding/binary"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"
)

// x32Insns returns the number of x32-check instructions buildFilter
// emits on the current arch: x32GuardLen on amd64, 0 on arm64.
func x32Insns() int {
	if rejectX32 {
		return x32GuardLen
	}
	return 0
}

// TestBuildFilter_Length verifies the instruction count matches the
// documented formula: len(dangerousSyscalls) + 6 + X instructions
// (arch LD, arch JEQ, syscall LD, optional x32 JGE, N JEQs,
// RET ALLOW, RET DENY, RET KILL).
func TestBuildFilter_Length(t *testing.T) {
	prog := buildFilter()
	require.Len(t, prog, len(dangerousSyscalls)+6+x32Insns())
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

	// pos 1: JEQ expectedArch, jt=0, jf=N+3+X
	require.Equal(t, uint16(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K), prog[1].Code, "prog[1].Code")
	require.Equal(t, expectedArch, prog[1].K, "prog[1].K")
	require.Equal(t, uint8(0), prog[1].Jt, "prog[1].Jt")
	require.Equal(t, uint8(len(dangerousSyscalls)+3+x32Insns()), prog[1].Jf, "prog[1].Jf (N+3+X)")
}

// TestBuildFilter_SyscallNrLoad verifies the instruction that loads
// seccomp_data.nr is at position 2, immediately after the arch
// prologue, and before the JEQ chain.
func TestBuildFilter_SyscallNrLoad(t *testing.T) {
	prog := buildFilter()
	require.Equal(t, uint16(unix.BPF_LD|unix.BPF_W|unix.BPF_ABS), prog[2].Code, "prog[2].Code")
	require.Equal(t, uint32(seccompDataNrOffset), prog[2].K, "prog[2].K")
}

// TestBuildFilter_X32Check verifies the x32 ABI guard. On amd64 the
// instructions right after LD [nr] must kill x32SyscallBit numbers and
// the legacy 512–547 range; on arm64 the JEQ chain must start there.
func TestBuildFilter_X32Check(t *testing.T) {
	prog := buildFilter()
	n := len(dangerousSyscalls)
	inst := prog[3]

	if runtime.GOARCH != "amd64" {
		require.Equal(t, uint16(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K), inst.Code, "prog[3].Code")
		require.Equal(t, dangerousSyscalls[0], inst.K, "prog[3].K")
		return
	}

	kill := n + 5 + x32GuardLen

	require.Equal(t, uint16(unix.BPF_JMP|unix.BPF_JGE|unix.BPF_K), inst.Code, "prog[3].Code")
	require.Equal(t, x32SyscallBit, inst.K, "prog[3].K")
	require.Equal(t, uint8(0), inst.Jf, "prog[3].Jf")
	require.Equal(t, kill, 3+1+int(inst.Jt), "prog[3] jt target (KILL)")

	skip := prog[4]
	require.Equal(t, uint16(unix.BPF_JMP|unix.BPF_JGT|unix.BPF_K), skip.Code, "prog[4].Code")
	require.Equal(t, x32LegacyLast, skip.K, "prog[4].K")
	require.Equal(t, uint8(0), skip.Jf, "prog[4].Jf")
	require.Equal(t, 6, 4+1+int(skip.Jt), "prog[4] jt target (first JEQ)")

	legacy := prog[5]
	require.Equal(t, uint16(unix.BPF_JMP|unix.BPF_JGE|unix.BPF_K), legacy.Code, "prog[5].Code")
	require.Equal(t, x32LegacyFirst, legacy.K, "prog[5].K")
	require.Equal(t, uint8(0), legacy.Jf, "prog[5].Jf")
	require.Equal(t, kill, 5+1+int(legacy.Jt), "prog[5] jt target (KILL)")

	require.Equal(t, retKill, prog[kill].K, "KILL position K")
}

// TestBuildFilter_JEQChain verifies each JEQ instruction in the
// denylist chain: instruction at position 3+X+i (i=0..N-1) compares
// against dangerousSyscalls[i] and jumps to DENY on match. The jt
// offset must be `N - i` so that (3+X+i) + 1 + jt = N+4+X = DENY.
func TestBuildFilter_JEQChain(t *testing.T) {
	prog := buildFilter()
	n := len(dangerousSyscalls)
	x := x32Insns()

	for i, nr := range dangerousSyscalls {
		pos := 3 + x + i
		inst := prog[pos]
		require.Equal(t, uint16(unix.BPF_JMP|unix.BPF_JEQ|unix.BPF_K), inst.Code, "prog[%d].Code", pos)
		require.Equal(t, nr, inst.K, "prog[%d].K", pos)
		wantJt := uint8(n - i)
		require.Equal(t, wantJt, inst.Jt, "prog[%d].Jt", pos)
		require.Equal(t, uint8(0), inst.Jf, "prog[%d].Jf", pos)

		// Sanity check that the computed jt actually lands on DENY.
		target := pos + 1 + int(inst.Jt)
		require.Equal(t, n+4+x, target, "prog[%d] jt target (DENY)", pos)
	}
}

// TestBuildFilter_ReturnInstructions verifies the three RET
// instructions at the tail of the program. Order matters: RET ALLOW
// must be the fallthrough target, RET ERRNO(EPERM) must be the DENY
// target, and RET KILL_PROCESS must be the arch-mismatch target.
func TestBuildFilter_ReturnInstructions(t *testing.T) {
	prog := buildFilter()
	n := len(dangerousSyscalls) + x32Insns()

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

	// Arch check is at position 1. On mismatch jf = N+3+X, so the
	// next executed instruction is at 1 + 1 + (N+3+X) = N+5+X = KILL.
	target := 1 + 1 + int(prog[1].Jf)
	wantTarget := n + 5 + x32Insns()
	require.Equal(t, wantTarget, target, "arch mismatch target (KILL)")

	// And KILL at position N+5 must actually be RET KILL_PROCESS.
	require.Equal(t, retKill, prog[wantTarget].K, "KILL position K")
}

// runFilter executes prog in the golang.org/x/net/bpf interpreter
// against a synthesized seccomp_data and returns the filter action.
// The interpreter loads 32-bit words big-endian while the kernel uses
// native byte order; encoding the words big-endian here keeps the
// loaded values identical, which is all the program inspects.
func runFilter(t *testing.T, prog []unix.SockFilter, arch, nr uint32) uint32 {
	t.Helper()

	raw := make([]bpf.RawInstruction, len(prog))
	for i, inst := range prog {
		raw[i] = bpf.RawInstruction{Op: inst.Code, Jt: inst.Jt, Jf: inst.Jf, K: inst.K}
	}
	insts, ok := bpf.Disassemble(raw)
	require.True(t, ok, "filter contains instructions the interpreter cannot decode")
	vm, err := bpf.NewVM(insts)
	require.NoError(t, err)

	// struct seccomp_data is 64 bytes: nr, arch, instruction_pointer, args[6].
	data := make([]byte, 64)
	binary.BigEndian.PutUint32(data[seccompDataNrOffset:], nr)
	binary.BigEndian.PutUint32(data[seccompDataArchOffset:], arch)

	ret, err := vm.Run(data)
	require.NoError(t, err)
	return uint32(ret)
}

// TestBuildFilter_Behavior runs the assembled program through a BPF
// interpreter and checks the action for representative inputs. Unlike
// the structural tests above it does not depend on the instruction
// layout, so it catches offset mistakes that a layout-mirroring test
// would replicate.
func TestBuildFilter_Behavior(t *testing.T) {
	t.Parallel()

	prog := buildFilter()

	type tc struct {
		name string
		arch uint32
		nr   uint32
		want uint32
	}
	cases := []tc{
		{"allowed syscall", expectedArch, uint32(unix.SYS_GETPID), retAllow},
		{"wrong arch", expectedArch ^ 0xFFFF, uint32(unix.SYS_GETPID), retKill},
		{"wrong arch denylisted nr", expectedArch ^ 0xFFFF, uint32(unix.SYS_MOUNT), retKill},
	}
	for _, nr := range dangerousSyscalls {
		cases = append(cases, tc{"denylisted", expectedArch, nr, retDeny})
	}

	// x32 ABI syscalls report the native arch but set x32SyscallBit,
	// or (before Linux 5.4) use the legacy x32 range 512–547 without
	// it. On amd64 every one of them, denylisted or not, must be
	// killed; on arm64 these numbers have no special meaning and are
	// simply not in the denylist. Keyed on GOARCH, not rejectX32, so
	// that disabling the check on amd64 fails this test.
	x32Want := retKill
	if runtime.GOARCH != "amd64" {
		x32Want = retAllow
	}
	cases = append(cases,
		tc{"x32 mount", expectedArch, x32SyscallBit | uint32(unix.SYS_MOUNT), x32Want},
		tc{"x32 bpf", expectedArch, x32SyscallBit | uint32(unix.SYS_BPF), x32Want},
		tc{"x32 getpid", expectedArch, x32SyscallBit | uint32(unix.SYS_GETPID), x32Want},
		tc{"x32 bit only", expectedArch, x32SyscallBit, x32Want},
		tc{"max nr", expectedArch, 0xFFFFFFFF, x32Want},
		tc{"just below x32 bit", expectedArch, x32SyscallBit - 1, retAllow},
		tc{"legacy x32 first", expectedArch, x32LegacyFirst, x32Want},
		tc{"legacy x32 ptrace", expectedArch, 521, x32Want},
		tc{"legacy x32 last", expectedArch, x32LegacyLast, x32Want},
		tc{"below legacy x32 range", expectedArch, x32LegacyFirst - 1, retAllow},
		tc{"above legacy x32 range", expectedArch, x32LegacyLast + 1, retAllow},
	)

	for _, c := range cases {
		require.Equal(t, c.want, runFilter(t, prog, c.arch, c.nr),
			"%s: arch=%#x nr=%#x", c.name, c.arch, c.nr)
	}
}

// TestDangerousSyscalls_ExpectedCount is a cheap sanity check that
// the denylist has the expected size. A future edit that accidentally
// deletes an entry (merge conflict resolution, rebase, typo) would
// silently weaken the filter; this test makes that loud.
func TestDangerousSyscalls_ExpectedCount(t *testing.T) {
	require.Len(t, dangerousSyscalls, 23, "if the change is intentional, update this test and the package README")
}

// TestDangerousSyscalls_RequiredMembership asserts that every
// security-critical entry that the package's threat model promises to
// block is actually present in the denylist. The count test above
// catches deletions but not substitutions: a refactor that swaps
// SYS_BPF for SYS_OPEN would keep the count at 23 and silently let
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
		{"SYS_KEXEC_LOAD", uint32(unix.SYS_KEXEC_LOAD)},
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
