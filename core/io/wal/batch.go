// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package wal

import (
	"encoding/binary"
	"sync"
)

// appendBatch accumulates encoded records for one group commit and carries
// each record's result back to its Append. All fields are guarded by WAL.mu.
type appendBatch struct {
	buf    []byte // concatenated [len][crc][payload] records
	starts []int  // starts[i] is record i's offset in buf; one extra end entry
	offs   []Offset
	errs   []error
	refs   int // Appends that have not read their result yet
}

var appendBatchPool = sync.Pool{New: func() any { return &appendBatch{starts: []int{0}} }}

func getAppendBatch() *appendBatch {
	return appendBatchPool.Get().(*appendBatch) //nolint:errcheck // pool holds only *appendBatch
}

// putAppendBatch recycles b once every Append has read its result.
func putAppendBatch(b *appendBatch) {
	b.buf = b.buf[:0]
	b.starts = b.starts[:1]
	clear(b.errs)
	b.offs, b.errs = b.offs[:0], b.errs[:0]
	appendBatchPool.Put(b)
}

// add encodes one record and returns its index in the batch.
func (b *appendBatch) add(payload []byte, crc uint32) int {
	b.buf = binary.LittleEndian.AppendUint32(b.buf, uint32(len(payload)))
	b.buf = binary.LittleEndian.AppendUint32(b.buf, crc)
	b.buf = append(b.buf, payload...)
	b.starts = append(b.starts, len(b.buf))
	b.offs = append(b.offs, Offset{})
	b.errs = append(b.errs, nil)
	b.refs++
	return len(b.offs) - 1
}

// len returns the number of records in the batch.
func (b *appendBatch) len() int { return len(b.offs) }

// size returns the encoded size of record i.
func (b *appendBatch) size(i int) int64 { return int64(b.starts[i+1] - b.starts[i]) }

// fail records err for records [i, j).
func (b *appendBatch) fail(i, j int, err error) {
	for k := i; k < j; k++ {
		b.errs[k] = err
	}
}
