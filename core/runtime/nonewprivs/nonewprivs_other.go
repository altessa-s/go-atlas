// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build !linux

package nonewprivs

// set is the non-Linux stub for [Set]. PR_SET_NO_NEW_PRIVS is a Linux
// prctl option with no portable equivalent; on darwin, windows, freebsd,
// and every other platform it simply returns [ErrUnsupported].
func set() error {
	return ErrUnsupported
}

// enabled is the non-Linux stub for [Enabled]. It always returns
// (false, [ErrUnsupported]).
func enabled() (bool, error) {
	return false, ErrUnsupported
}
