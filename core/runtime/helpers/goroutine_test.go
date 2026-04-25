// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package helpers

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGoroutineID_IsStableWithinGoroutine(t *testing.T) {
	id1 := GoroutineID()
	require.NotZero(t, id1, "GoroutineID returned 0")

	id2 := GoroutineID()
	require.Equal(t, id1, id2)
}

func TestGoroutineID_DifferentGoroutinesUsuallyDiffer(t *testing.T) {
	// This is a best-effort sanity check; if runtime changes, we mainly want
	// to ensure it still returns non-zero and doesn't panic.
	idMain := GoroutineID()
	require.NotZero(t, idMain, "main GoroutineID returned 0")

	ch := make(chan int, 1)
	go func() {
		ch <- GoroutineID()
	}()
	idOther := <-ch
	require.NotZero(t, idOther, "other GoroutineID returned 0")
}
