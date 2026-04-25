// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package io

import (
	"bytes"
	"sync"
)

// BufferPool is a [sync.Pool] that manages reusable [bytes.Buffer] instances to reduce
// garbage collection pressure. Callers should prefer [GetBuffer] and [PutBuffer] over
// accessing BufferPool directly, since those helpers reset buffers and enforce a
// capacity threshold to prevent unbounded memory retention. BufferPool is safe for
// concurrent use.
var BufferPool = sync.Pool{
	New: func() any {
		return &bytes.Buffer{}
	},
}

// GetBuffer retrieves a [bytes.Buffer] from [BufferPool], resets it, and returns it
// ready for use. If the pool is empty or the retrieved value is not a *bytes.Buffer,
// a fresh buffer is allocated. The returned buffer is safe to write to immediately.
// Callers should return the buffer with [PutBuffer] when finished to enable reuse.
// This function is safe for concurrent use.
func GetBuffer() *bytes.Buffer {
	buf, ok := BufferPool.Get().(*bytes.Buffer)
	if !ok {
		buf = &bytes.Buffer{}
	}
	buf.Reset()
	return buf
}

// PutBuffer returns a [bytes.Buffer] to [BufferPool] for reuse after resetting it.
// If buf is nil, the call is a no-op. Buffers whose capacity exceeds 64 KB are
// silently discarded instead of being returned to the pool, preventing a single
// large allocation from persisting in the pool and consuming excessive memory.
// This function is safe for concurrent use.
func PutBuffer(buf *bytes.Buffer) {
	if buf == nil {
		return
	}
	// Don't return extremely large buffers to the pool to avoid memory waste.
	// 64KB is a reasonable threshold for reuse.
	if buf.Cap() > 64*1024 {
		return
	}
	buf.Reset()
	BufferPool.Put(buf)
}
