// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package helpers

import (
	"bytes"
	"runtime"
)

// stackBufferSizeGoroutine is the buffer size for reading goroutine stack.
const stackBufferSizeGoroutine = 64

const (
	goroutinePrefix = "goroutine "
	intBase10       = 10
)

// GoroutineID returns the numeric ID of the calling goroutine, or 0 if it
// cannot be determined. The ID is extracted by parsing the output of
// [runtime.Stack], which is an unsupported implementation detail of the Go
// runtime and may change between Go releases.
//
// This function allocates a small stack buffer on each call and should not be
// used in performance-critical paths.
//
// Example:
//
//	id := helpers.GoroutineID()
func GoroutineID() int {
	var buf [stackBufferSizeGoroutine]byte
	n := runtime.Stack(buf[:], false)
	b := buf[:n]

	// Expected first line prefix: "goroutine <id> ["
	if !bytes.HasPrefix(b, []byte(goroutinePrefix)) {
		return 0
	}

	id := 0
	for i := len(goroutinePrefix); i < len(b); i++ {
		ch := b[i]
		if ch < '0' || ch > '9' {
			return id
		}
		id = id*intBase10 + int(ch-'0')
	}

	return id
}
