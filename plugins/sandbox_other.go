// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build !linux

package plugins

// applySandbox is the non-Linux stub for the plugin sandbox. The Linux
// process-hardening primitives ([unix.Prctl], [syscall.Setrlimit] for
// PR_SET_NO_NEW_PRIVS / RLIMIT_AS / RLIMIT_NPROC / RLIMIT_NOFILE) are not
// portable. On any non-Linux platform an enabled sandbox is rejected at
// [Manager.Load] time with [ErrSandboxUnsupported]; a disabled sandbox is
// a no-op like everywhere else so cross-platform builds work without
// behavior changes.
func applySandbox(o SandboxOptions) error {
	if !o.Enabled {
		return nil
	}
	return ErrSandboxUnsupported
}
