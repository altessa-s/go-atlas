// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package checksum verifies policy source manifests.
//
// # Ownership
//
// Manifests snapshot their input and preserve source-specific error identities.
//
//	m := checksum.New(expected)
//	err := m.Verify(path, content, errMismatch, errUnexpected)
package checksum
