// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ptr_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/core/types/ptr"
)

func TestWrap(t *testing.T) {
	val := 42
	p := ptr.Wrap(val)
	if p == nil || *p != val {
		t.Error("Wrap failed")
	}
}

func TestWrapNonZero(t *testing.T) {
	if ptr.WrapNonZero(0) != nil {
		t.Error("WrapNonZero(0) should be nil")
	}
	if p := ptr.WrapNonZero(42); p == nil || *p != 42 {
		t.Error("WrapNonZero(42) failed")
	}
}

func TestUnwrap(t *testing.T) {
	val := 42
	if ptr.Unwrap(&val) != 42 {
		t.Error("Unwrap(&val) failed")
	}
	if ptr.Unwrap[int](nil) != 0 {
		t.Error("Unwrap(nil) should be zero")
	}
	if ptr.Unwrap[int](nil, 123) != 123 {
		t.Error("Unwrap(nil, default) failed")
	}
}
