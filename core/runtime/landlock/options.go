// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package landlock

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

// options configures a Landlock ruleset. Populated via [Option] functions
// passed to [Apply]. Every path must be a non-empty absolute filesystem
// path; [Apply] validates that invariant before invoking any syscalls.
type options struct {
	// readPaths grants read+execute access on each listed path (and
	// everything beneath it for directories). Use [WithReadPaths] to set.
	readPaths []string `optgen:"append"`

	// readWritePaths grants read+execute+write+truncate access on each
	// listed path. On kernels older than Landlock ABI 3 the truncate flag
	// is silently dropped from the ruleset. Use [WithReadWritePaths] to set.
	readWritePaths []string `optgen:"append"`
}
