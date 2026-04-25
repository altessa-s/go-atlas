// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

import "io"

// MockReadCloser wraps any [io.Reader] as an [io.ReadCloser] and records
// whether [MockReadCloser.Close] was called via the Closed field.
type MockReadCloser struct {
	io.Reader
	Closed bool
}

// Close implements io.Closer.
func (m *MockReadCloser) Close() error {
	m.Closed = true
	return nil
}
