// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package landlock_test

import (
	"errors"
	"log"

	"github.com/altessa-s/go-atlas/core/runtime/landlock"
)

// ExampleApply demonstrates the canonical usage of [landlock.Apply] from
// an external consumer. It is not executed (no "Output:" directive) because
// Apply is irreversible and would restrict every subsequent test in the
// binary; instead, the example is compile-checked so that any future
// change to the Apply signature breaks this file and forces a review.
func ExampleApply() {
	err := landlock.Apply(
		landlock.WithReadPaths("/etc", "/lib64", "/usr/lib64"),
		landlock.WithReadWritePaths("/var/log/myservice"),
	)
	switch {
	case err == nil:
		// Filesystem ruleset is now in force for the process.
	case errors.Is(err, landlock.ErrUnsupported):
		log.Print("landlock: kernel does not support Landlock; continuing unsandboxed")
	case errors.Is(err, landlock.ErrInvalidOption):
		log.Fatalf("landlock: bad operator config: %v", err)
	case errors.Is(err, landlock.ErrFailed):
		log.Fatalf("landlock: kernel rejected the ruleset: %v", err)
	default:
		log.Fatalf("landlock: unexpected error: %v", err)
	}
}
