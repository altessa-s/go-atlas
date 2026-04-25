// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package rlimits_test

import (
	"errors"
	"log"

	"github.com/altessa-s/go-atlas/core/runtime/rlimits"
)

// ExampleApply demonstrates the canonical usage of [rlimits.Apply] from
// an external consumer. It is not executed (no "Output:" directive)
// because lowered hard limits are irreversible and would propagate to
// every subsequent test in the binary; instead, the example is
// compile-checked so that any future change to the Apply signature
// breaks this file and forces a review.
func ExampleApply() {
	err := rlimits.Apply(
		rlimits.WithMemoryBytes(512<<20),
		rlimits.WithMaxOpenFiles(4096),
		rlimits.WithMaxProcesses(256),
		rlimits.WithDisableCoreDumps(),
	)
	switch {
	case err == nil:
		// Resource caps are in place for the process.
	case errors.Is(err, rlimits.ErrUnsupported):
		log.Print("rlimits: platform does not support setrlimit; continuing unsandboxed")
	case errors.Is(err, rlimits.ErrInvalidOption):
		log.Fatalf("rlimits: bad operator config: %v", err)
	case errors.Is(err, rlimits.ErrFailed):
		log.Fatalf("rlimits: kernel rejected the setrlimit call: %v", err)
	default:
		log.Fatalf("rlimits: unexpected error: %v", err)
	}
}
