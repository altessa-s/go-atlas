// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spool

import (
	"bytes"
	"errors"
	"io"
	"os"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// ErrTooLarge is returned by [New] when the source exceeds the cap set via
// [WithMaxBytes]. Callers can test for it with [errors.Is].
var ErrTooLarge = errors.New("spool: content exceeds max bytes")

// Spool is a local, rewindable copy of a source reader. [New] materializes
// the source exactly once — in memory for small content, in a temp file for
// larger content — so callers can re-read it from offset 0 without touching
// the (possibly remote) origin. Seek is a cheap local operation; Close
// releases the backing store. A Spool is not safe for concurrent use.
type Spool struct {
	io.ReadSeeker
	size    int64
	cleanup func() error
}

// New drains r once into a local backing store and returns a rewindable
// reader positioned at offset 0. The source reader is consumed in full and is
// not closed — the caller owns its lifetime. The returned [Spool] must be
// Closed to release a spilled temp file.
func New(r io.Reader, opts ...Option) (*Spool, error) {
	o := newOptions(opts...)

	// Read one byte past the smaller of the memory threshold and the cap: that
	// is enough to decide memory-vs-spill and to detect an over-cap source.
	firstLimit := o.memThreshold + 1
	if o.maxBytes > 0 && o.maxBytes+1 < firstLimit {
		firstLimit = o.maxBytes + 1
	}

	// headDst tees into the observer (when set) so the digest sees every byte
	// written to the in-memory buffer.
	var head bytes.Buffer
	var headDst io.Writer = &head
	if o.tee != nil {
		headDst = io.MultiWriter(&head, o.tee)
	}
	n, err := io.CopyN(headDst, r, firstLimit)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, coreerrs.WrapOperation(err, "read into spool")
	}
	if o.maxBytes > 0 && n > o.maxBytes {
		return nil, ErrTooLarge
	}
	if n <= o.memThreshold {
		return &Spool{ReadSeeker: bytes.NewReader(head.Bytes()), size: n}, nil
	}

	f, err := os.CreateTemp("", "spool-*")
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create spool file")
	}
	remove := func() error { return errors.Join(f.Close(), os.Remove(f.Name())) }
	// The head is already teed; write it to the file without re-teeing.
	if _, err = f.Write(head.Bytes()); err != nil {
		return nil, errors.Join(coreerrs.WrapOperation(err, "spill spool"), remove())
	}

	// fileDst tees the remainder of the source into the observer.
	var fileDst io.Writer = f
	if o.tee != nil {
		fileDst = io.MultiWriter(f, o.tee)
	}

	var m int64
	if o.maxBytes > 0 {
		rem := o.maxBytes - n
		m, err = io.CopyN(fileDst, r, rem+1)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, errors.Join(coreerrs.WrapOperation(err, "spill spool"), remove())
		}
		if m > rem {
			return nil, errors.Join(ErrTooLarge, remove())
		}
	} else if m, err = io.Copy(fileDst, r); err != nil {
		return nil, errors.Join(coreerrs.WrapOperation(err, "spill spool"), remove())
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, errors.Join(coreerrs.WrapOperation(err, "rewind spool"), remove())
	}
	return &Spool{ReadSeeker: f, size: n + m, cleanup: remove}, nil
}

// Size returns the total number of bytes materialized into the spool.
func (s *Spool) Size() int64 { return s.size }

// Close releases the spool's backing store. It is a no-op for an in-memory
// spool, removes the temp file for a spilled one, and is idempotent.
func (s *Spool) Close() error {
	if s.cleanup == nil {
		return nil
	}
	cleanup := s.cleanup
	s.cleanup = nil
	return cleanup()
}
