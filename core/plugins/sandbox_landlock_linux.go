// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build linux

package plugins

import (
	"github.com/altessa-s/go-atlas/core/runtime/landlock"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// applyLandlock is the plugin sandbox adapter over [landlock.Apply]. It
// translates plugin-local [LandlockOptions] into [landlock.Config], calls
// the standalone runtime package, and wraps any error with
// [ErrSandboxFailed] so callers can still match on the plugin-level
// sentinel via [errors.Is].
//
// The PR_SET_NO_NEW_PRIVS prerequisite is satisfied by [applySandbox]
// earlier in the pipeline (SandboxOptions.NoNewPrivs defaults to true
// and runs before this function). Operators who disable noNewPrivs and
// enable Landlock without CAP_SYS_ADMIN see a clear hint in the returned
// error, forwarded from the landlock package.
func applyLandlock(o LandlockOptions) error {
	if !o.Enabled {
		return nil
	}
	err := landlock.Apply(
		landlock.WithReadPaths(o.ReadPaths...),
		landlock.WithReadWritePaths(o.ReadWritePaths...),
	)
	if err != nil {
		return coreerrs.Wrapf(ErrSandboxFailed, "landlock: %v", err)
	}
	return nil
}
