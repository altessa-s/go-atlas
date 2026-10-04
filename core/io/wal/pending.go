// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package wal

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"os"
)

// ErrCorruptRecord means a pending record failed its length or checksum check.
var ErrCorruptRecord = errors.New("wal: corrupt pending record")

// ReadPending reads up to limit unacknowledged records in append order. skip
// may exclude records already being delivered; it runs under the WAL lock and
// must not block or call WAL methods. Memory is bounded by the selected records.
// Acknowledgements are process-local; after restart an incompletely acknowledged
// segment may replay already delivered records.
func (w *WAL) ReadPending(limit int, skip func(Offset) bool) ([]Record, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed.Load() {
		return nil, ErrClosed
	}
	if limit <= 0 {
		return nil, nil
	}
	var records []Record
	for _, seg := range w.sealed {
		if err := w.readPendingSegment(seg, limit, skip, &records); err != nil {
			return nil, err
		}
		if len(records) == limit {
			return records, nil
		}
	}
	if w.active != nil {
		if err := w.readPendingSegment(w.active, limit, skip, &records); err != nil {
			return nil, err
		}
	}
	return records, nil
}

func (w *WAL) readPendingSegment(seg *segmentRef, limit int, skip func(Offset) bool, out *[]Record) error {
	if seg.unacked == 0 {
		return nil
	}
	f, err := os.Open(seg.path)
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck // read-only descriptor
	var header [recordHeaderSize]byte
	for pos := seg.pendingPosition; pos < seg.size && len(*out) < limit; {
		if _, err := f.ReadAt(header[:], pos); err != nil {
			return err
		}
		n := int64(binary.LittleEndian.Uint32(header[:4]))
		next := pos + recordHeaderSize + n
		if n == 0 || n > maxRecordBytes || next > seg.size {
			return ErrCorruptRecord
		}
		offset := Offset{SegmentID: seg.id, Position: pos}
		_, acked := seg.acked[pos]
		if acked && pos == seg.pendingPosition {
			delete(seg.acked, pos)
			seg.pendingPosition = next
		}
		if !acked && (skip == nil || !skip(offset)) {
			payload := make([]byte, n)
			if _, err := io.ReadFull(io.NewSectionReader(f, pos+recordHeaderSize, n), payload); err != nil {
				return err
			}
			if crc32.ChecksumIEEE(payload) != binary.LittleEndian.Uint32(header[4:]) {
				return ErrCorruptRecord
			}
			*out = append(*out, Record{Offset: offset, Payload: payload})
		}
		pos = next
	}
	return nil
}

func (s *segmentRef) ack(position int64) {
	if position < s.pendingPosition || position < 0 || position >= s.size || s.unacked == 0 {
		return
	}
	if _, ok := s.acked[position]; ok {
		return
	}
	if s.acked == nil {
		s.acked = make(map[int64]struct{})
	}
	s.acked[position] = struct{}{}
	s.unacked--
}
