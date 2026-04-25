// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package rlimits

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate --type=options

// options configures a batch of rlimits. Populated via [Option] functions
// passed to [Apply]. Zero values leave the kernel default in place for
// that resource; the only exception is disableCoreDumps, which is a
// boolean because pinning RLIMIT_CORE to zero is itself a meaningful
// operation distinct from "leave it alone".
type options struct {
	// memoryBytes caps RLIMIT_AS (virtual address space). 0 = unset.
	memoryBytes int64

	// maxOpenFiles caps RLIMIT_NOFILE. 0 = unset.
	maxOpenFiles int64

	// maxProcesses caps RLIMIT_NPROC (processes per real UID). 0 = unset.
	maxProcesses int64

	// maxFileSizeBytes caps RLIMIT_FSIZE. 0 = unset.
	maxFileSizeBytes int64

	// disableCoreDumps pins RLIMIT_CORE to 0 so the process cannot leak
	// memory contents (including secrets) via a post-crash core dump.
	disableCoreDumps bool
}
