// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package exectest provides a helper for tests that need to execute
// syscalls with irreversible process-wide side effects — Linux security
// primitives like capset(2), prctl(PR_SET_NO_NEW_PRIVS),
// landlock_restrict_self(2), seccomp(SET_MODE_FILTER), and persistent
// setrlimit calls. The kernel does not let a process undo any of those
// once applied, so running them in the test binary itself would
// contaminate every later test.
//
// The helper re-execs the test binary in a subprocess so the side
// effect is contained to a child that exits immediately afterward.
// Use it like this:
//
//	func TestDropAllSubprocess(t *testing.T) {
//	    exectest.RunInSubprocess(t, func() {
//	        if err := capabilities.DropAll(); err != nil {
//	            exectest.Failf("DropAll: %v", err)
//	        }
//	        snap, err := capabilities.Get()
//	        if err != nil {
//	            exectest.Failf("Get after DropAll: %v", err)
//	        }
//	        if snap.Effective != 0 {
//	            exectest.Failf("Effective != 0 after DropAll: %#x", snap.Effective)
//	        }
//	    })
//	}
//
// The same test function plays both roles. The parent invocation
// (under `go test`) re-execs the binary with -test.run=^TestName$ and
// the GO_ATLAS_EXECTEST_CHILD env var set; the child invocation
// detects the env var and runs fn directly. Standard testing.T methods
// do NOT cross the process boundary — use [Failf] inside fn to report
// failures.
//
// RunInSubprocess only runs on Linux; on every other platform it calls
// t.Skip. Tests that depend on this helper should live in
// `_linux_test.go` files so the build never compiles them on macOS or
// Windows.
package exectest

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"
)

// childEnvVar is the environment variable the parent sets to mark a
// subprocess invocation. The child uses its presence to switch from
// the standard test-binary entry path into the dangerous-syscall path.
const childEnvVar = "GO_ATLAS_EXECTEST_CHILD"

// childRanSentinel is written to stdout by the child immediately
// before invoking fn. The parent asserts this string appears in the
// child's combined output, which protects against the failure mode
// where -test.run filters out every test (e.g. because the test name
// changed) and the child exits 0 with "no tests to run" — without
// the sentinel that would look like a passing test.
const childRanSentinel = "GO_ATLAS_EXECTEST_CHILD_RAN"

// Exit codes used by the child half of RunInSubprocess to tell the
// parent what happened. Kept as named constants so the meaning is
// obvious at call sites and the parent can (eventually) branch on
// them.
const (
	// exitCodeSuccess is returned when fn completed without calling
	// Failf and without panicking.
	exitCodeSuccess = 0

	// exitCodeFailure is used by Failf to signal that an in-child
	// assertion failed with a well-formed error message on stderr.
	exitCodeFailure = 1

	// exitCodePanic is used when fn panicked. The recovered value is
	// printed to stderr and surfaced by the parent via t.Fatalf.
	exitCodePanic = 2
)

// RunInSubprocess runs fn in a child process by re-execing the current
// test binary with -test.run set to the calling test's name. The
// parent half waits for the child and fails the test if the child
// exited non-zero or did not emit the sentinel that proves fn ran.
//
// In the child process, fn is called directly. Use [Failf] (not
// testing.T methods) to report failures from inside fn — the t passed
// to RunInSubprocess in the child is not connected to the parent's
// reporting.
//
// On non-Linux platforms RunInSubprocess calls t.Skip immediately,
// which is the right behavior for tests of Linux-only primitives.
func RunInSubprocess(t *testing.T, fn func()) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skipf("subprocess test requires linux, current GOOS=%s", runtime.GOOS)
	}
	if os.Getenv(childEnvVar) == "1" {
		// runChildBody returns the exit code rather than calling
		// os.Exit itself so its deferred recover is guaranteed to
		// run — os.Exit bypasses deferred functions, so the recover
		// must come from a scope that has already returned.
		os.Exit(runChildBody(fn))
	}
	runParent(t)
}

// runChildBody executes fn in the child half of [RunInSubprocess]. It
// returns the exit code that the caller should pass to [os.Exit]. The
// deferred recover captures panics from fn and maps them to
// [exitCodePanic]; the caller must be careful not to os.Exit inside
// this function, because that would skip the recover.
func runChildBody(fn func()) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "exectest: child panicked: %v\n", r)
			code = exitCodePanic
		}
	}()
	// Print the sentinel BEFORE fn so even a fast-exiting fn (which
	// might happen if it calls os.Exit internally) leaves evidence
	// that the child reached the test body. Fprintln errors are
	// ignored on purpose: a broken stdout in a sandboxing test would
	// cause a second os.Exit which we do not want racing with the
	// deferred recover above.
	_, _ = fmt.Fprintln(os.Stdout, childRanSentinel)
	fn()
	return exitCodeSuccess
}

// runParent re-execs the test binary, waits for the child, and
// asserts the result. Called from the parent invocation only.
func runParent(t *testing.T) {
	t.Helper()
	testName := t.Name()
	// -test.run anchors prevent prefix matches like "TestFoo" matching
	// "TestFooBar" — without ^...$ a test rename could silently run
	// the wrong child. -test.v makes child output easier to debug
	// when a test fails in CI. CommandContext is required by the
	// `noctx` linter and ties the child's lifetime to the test's
	// context so a timed-out parent test kills its child instead of
	// leaking it.
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^"+testName+"$", "-test.v")
	cmd.Env = append(os.Environ(), childEnvVar+"=1")
	out, err := cmd.CombinedOutput()
	if !bytes.Contains(out, []byte(childRanSentinel)) {
		t.Fatalf("child process did not run the test body (sentinel %q not found in output)\n"+
			"this usually means -test.run filtered out the test or the binary exited before fn\n"+
			"--- child output ---\n%s",
			childRanSentinel, out)
	}
	if err != nil {
		t.Fatalf("child process failed: %v\n--- child output ---\n%s", err, out)
	}
}

// Failf reports a failure from inside the function passed to
// [RunInSubprocess]. It writes to stderr (which the parent surfaces in
// its t.Fatalf message) and exits the child with [exitCodeFailure].
//
// Failf must only be called from a child process. Calling it from
// non-test code or outside RunInSubprocess will terminate whatever
// process invokes it.
//
// Note: Failf calls os.Exit directly, which bypasses deferred
// functions — including the panic recovery in [runChildBody]. This is
// intentional: a child that calls Failf is not panicking, it is
// cleanly reporting an assertion failure, and we want the exit code
// to be [exitCodeFailure] rather than [exitCodePanic]. Any cleanup
// that a child needs on failure should be done before calling Failf.
func Failf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "exectest: child failure: "+format+"\n", args...)
	os.Exit(exitCodeFailure)
}
