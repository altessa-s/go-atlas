// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import "testing"

func BenchmarkStackTrace(b *testing.B) {
	for b.Loop() {
		StackTrace(0)
	}
}

func BenchmarkNewPanicError(b *testing.B) {
	for b.Loop() {
		NewPanicError("panic", 0)
	}
}

func BenchmarkFrames_String(b *testing.B) {
	frames := Frames{
		{File: "a.go", Line: 10, Function: "Foo"},
		{File: "b.go", Line: 20, Function: "Bar"},
		{File: "c.go", Line: 30, Function: "Baz"},
	}
	var s string
	for b.Loop() {
		s = frames.String()
	}
	_ = s
}

func BenchmarkFrames_MarshalJSON(b *testing.B) {
	frames := Frames{
		{File: "a.go", Line: 10, Function: "Foo"},
		{File: "b.go", Line: 20, Function: "Bar"},
	}
	for b.Loop() {
		frames.MarshalJSON()
	}
}
