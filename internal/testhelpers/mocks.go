// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package testhelpers

// MockNetError implements the [net.Error] interface for testing.
// The exported fields control the return values of the corresponding methods:
// Msg is returned by Error, IsTimeout by Timeout, and IsTemp by Temporary.
type MockNetError struct {
	IsTimeout bool
	IsTemp    bool
	Msg       string
}

func (e *MockNetError) Error() string   { return e.Msg }
func (e *MockNetError) Timeout() bool   { return e.IsTimeout }
func (e *MockNetError) Temporary() bool { return e.IsTemp }

// --- MockIdempotencyStorage ---
