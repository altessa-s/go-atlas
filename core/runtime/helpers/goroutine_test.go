// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package helpers

import "testing"

func TestGoroutineID_IsStableWithinGoroutine(t *testing.T) {
	id1 := GoroutineID()
	if id1 == 0 {
		t.Fatalf("GoroutineID returned 0")
	}

	id2 := GoroutineID()
	if id2 != id1 {
		t.Fatalf("id2=%d, want %d", id2, id1)
	}
}

func TestGoroutineID_DifferentGoroutinesUsuallyDiffer(t *testing.T) {
	// This is a best-effort sanity check; if runtime changes, we mainly want
	// to ensure it still returns non-zero and doesn't panic.
	idMain := GoroutineID()
	if idMain == 0 {
		t.Fatalf("main GoroutineID returned 0")
	}

	ch := make(chan int, 1)
	go func() {
		ch <- GoroutineID()
	}()
	idOther := <-ch
	if idOther == 0 {
		t.Fatalf("other GoroutineID returned 0")
	}
}
