// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spool

import (
	"bytes"
	"errors"
	"io"
	"os"
	"sync"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// lrPool reuses *io.LimitedReader values so New avoids one heap allocation per
// call — io.CopyN allocates a fresh LimitedReader internally on every call.
var lrPool = sync.Pool{New: func() any { return new(io.LimitedReader) }}

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
	// inMem is the embedded bytes.Reader for in-memory spools. Embedding it
	// here avoids a separate heap allocation; file-backed spools leave it at
	// its zero value.
	inMem   bytes.Reader
	size    int64
	cleanup func() error
}

// New drains r once into a local backing store and returns a rewindable
// reader positioned at offset 0. The source reader is consumed in full and is
// not closed — the caller owns its lifetime. The returned [Spool] must be
// Closed to release a spilled temp file.
func New(r io.Reader, opts ...Option) (*Spool, error) {
	o := newOptions(opts...)
	if o.tee == nil {
		return newNoTee(r, o)
	}
	return newWithTee(r, o)
}

// newNoTee is the fast path for [New] when no tee writer is configured.
// head bytes.Buffer lives in its own function scope where &head is never
// converted to io.Writer, so the compiler's escape analysis keeps head on
// the stack and saves one heap allocation per call.
func newNoTee(r io.Reader, o *options) (*Spool, error) {
	// Read one byte past the smaller of the memory threshold and the cap: that
	// is enough to decide memory-vs-spill and to detect an over-cap source.
	firstLimit := o.memThreshold + 1
	if o.maxBytes > 0 && o.maxBytes+1 < firstLimit {
		firstLimit = o.maxBytes + 1
	}

	var head bytes.Buffer
	head.Grow(min(int(firstLimit), 4096))

	lr := lrPool.Get().(*io.LimitedReader)
	lr.R, lr.N = r, firstLimit
	n, err := head.ReadFrom(lr)
	lr.R = nil
	lrPool.Put(lr)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "read into spool")
	}
	if o.maxBytes > 0 && n > o.maxBytes {
		return nil, ErrTooLarge
	}
	if n <= o.memThreshold {
		sp := &Spool{size: n}
		sp.inMem.Reset(head.Bytes())
		sp.ReadSeeker = &sp.inMem
		return sp, nil
	}

	f, err := os.CreateTemp("", "spool-*")
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create spool file")
	}
	return spillToFile(r, head.Bytes(), f, nil, n, o.maxBytes)
}

// newWithTee handles [New] when a tee writer is configured.
func newWithTee(r io.Reader, o *options) (*Spool, error) {
	firstLimit := o.memThreshold + 1
	if o.maxBytes > 0 && o.maxBytes+1 < firstLimit {
		firstLimit = o.maxBytes + 1
	}

	var head bytes.Buffer
	head.Grow(min(int(firstLimit), 4096))

	lr := lrPool.Get().(*io.LimitedReader)
	lr.R, lr.N = r, firstLimit
	n, err := io.Copy(io.MultiWriter(&head, o.tee), lr)
	lr.R = nil
	lrPool.Put(lr)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "read into spool")
	}
	if o.maxBytes > 0 && n > o.maxBytes {
		return nil, ErrTooLarge
	}
	if n <= o.memThreshold {
		sp := &Spool{size: n}
		sp.inMem.Reset(head.Bytes())
		sp.ReadSeeker = &sp.inMem
		return sp, nil
	}

	f, err := os.CreateTemp("", "spool-*")
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create spool file")
	}
	return spillToFile(r, head.Bytes(), f, o.tee, n, o.maxBytes)
}

// spillToFile writes headBytes to f, copies the remainder of r into f (teeing
// into tee when non-nil), seeks f to offset 0, and returns the file-backed
// Spool. On any error it closes and removes f.
func spillToFile(r io.Reader, headBytes []byte, f *os.File, tee io.Writer, n, maxBytes int64) (*Spool, error) {
	remove := func() error { return errors.Join(f.Close(), os.Remove(f.Name())) }
	// The head is already teed (when a tee is set); write it to the file only.
	if _, err := f.Write(headBytes); err != nil {
		return nil, errors.Join(coreerrs.WrapOperation(err, "spill spool"), remove())
	}
	var fileDst io.Writer = f
	if tee != nil {
		fileDst = io.MultiWriter(f, tee)
	}
	var (
		m   int64
		err error
	)
	if maxBytes > 0 {
		rem := maxBytes - n
		lr := lrPool.Get().(*io.LimitedReader)
		lr.R, lr.N = r, rem+1
		m, err = io.Copy(fileDst, lr)
		lr.R = nil
		lrPool.Put(lr)
		if err != nil {
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
