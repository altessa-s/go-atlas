// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package runtime

import "testing"

func TestAddCleanup(t *testing.T) {
	obj := new(int)
	called := false
	cleanup := AddCleanup(obj, func(v bool) { called = v }, true)
	if cleanup == nil {
		t.Fatal("expected non-nil Cleanup")
	}
	// Stop should not panic
	cleanup.Stop()
	_ = called
}

func TestClearFinalizer(t *testing.T) {
	obj := new(int)
	// Should not panic
	ClearFinalizer(obj)
}
