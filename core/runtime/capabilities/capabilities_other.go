// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:build !linux

package capabilities

// getSnapshot is the non-Linux stub for [Get]. Linux capabilities
// have no portable equivalent; on darwin, windows, freebsd, and every
// other platform the snapshot is always empty and the call returns
// [ErrUnsupported].
func getSnapshot() (Sets, error) {
	return Sets{}, ErrUnsupported
}

// applySnapshot is the non-Linux stub for [Apply].
func applySnapshot(_ Sets) error {
	return ErrUnsupported
}

// dropAllExcept is the non-Linux stub for [DropAll] /
// [DropAllExcept].
func dropAllExcept(_ []Cap) error {
	return ErrUnsupported
}

// boundingDrop is the non-Linux stub for [BoundingDrop].
func boundingDrop(_ Cap) error {
	return ErrUnsupported
}

// ambientClearAll is the non-Linux stub for [AmbientClearAll].
func ambientClearAll() error {
	return ErrUnsupported
}

// ambientRaise is the non-Linux stub for [AmbientRaise].
func ambientRaise(_ Cap) error {
	return ErrUnsupported
}

// ambientLower is the non-Linux stub for [AmbientLower].
func ambientLower(_ Cap) error {
	return ErrUnsupported
}
