// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package recovery

import "testing"

func FuzzPanicError_Error(f *testing.F) {
	f.Add("panic message")
	f.Add("")
	f.Add("nil pointer dereference")

	f.Fuzz(func(t *testing.T, msg string) {
		pe := &PanicError{Panic: msg}
		got := pe.Error()
		if got == "" {
			t.Fatal("Error() returned empty string")
		}
	})
}

func FuzzShortname(f *testing.F) {
	f.Add("github.com/foo/bar.Func")
	f.Add("main.Func")
	f.Add("Func")
	f.Add("")

	f.Fuzz(func(t *testing.T, name string) {
		result := shortname(name)
		if len(result) > len(name) {
			t.Fatalf("shortname result longer than input")
		}
	})
}
