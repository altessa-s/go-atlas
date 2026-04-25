// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build !linux

package landlock

// apply is the non-Linux stub for the [Apply] platform dispatch. Landlock
// is a Linux LSM with no portable equivalent; on darwin, windows, freebsd,
// and every other platform it simply returns [ErrUnsupported]. Callers
// that want to fail-open on non-Linux should check [Supported] first
// rather than relying on Apply's error return.
func apply(_, _ []string) error {
	return ErrUnsupported
}

// Supported reports whether the running platform supports Landlock. It
// always returns false on non-Linux platforms because Landlock is a Linux
// LSM with no portable equivalent.
func Supported() bool {
	return false
}

// ABIVersion returns the highest Landlock ABI version supported by the
// running platform. It always returns (0, [ErrUnsupported]) on non-Linux
// platforms.
func ABIVersion() (int, error) {
	return 0, ErrUnsupported
}
