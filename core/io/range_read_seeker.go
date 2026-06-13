// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package io

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// Seek sentinel errors returned by [RangeReadSeeker.Seek]. Callers can test for
// these conditions with [errors.Is].
var (
	// ErrInvalidWhence is returned when Seek receives a whence value other than
	// io.SeekStart, io.SeekCurrent, or io.SeekEnd.
	ErrInvalidWhence = errors.New("invalid whence")

	// ErrNegativePosition is returned when Seek would move the position before
	// the start of the object.
	ErrNegativePosition = errors.New("negative position")
)

// RangeOpener opens a read of length bytes starting at offset within some
// underlying object. Implementations typically issue a ranged request to a
// remote store (S3 GetObject with a Range header, an HTTP Range request) or
// slice a local resource. The returned body must yield exactly length bytes;
// a shorter body surfaces as io.ErrUnexpectedEOF on the consuming reader.
type RangeOpener func(ctx context.Context, offset, length int64) (io.ReadCloser, error)

// RangeReadSeeker adapts a [RangeOpener] to io.ReadSeeker for consumers that
// need random access over a remote object without downloading it up front.
// Reads lazily open a ranged request at the current position; Seek only moves
// the position and forces the next Read to reopen, so seek-heavy consumers
// issue one ranged request per repositioning. The caller MUST Close it to
// release the open body, and must not use it from multiple goroutines.
//
// Construct instances with [NewRangeReadSeeker]; the zero value is not usable.
type RangeReadSeeker struct {
	// ctx is captured at construction: io.Reader has no context parameter, and
	// the adapter lives strictly within the request that created it.
	ctx  context.Context
	open RangeOpener
	size int64
	pos  int64
	body io.ReadCloser
}

var _ io.ReadSeekCloser = (*RangeReadSeeker)(nil)

// NewRangeReadSeeker returns a lazy io.ReadSeeker over an object served by
// open. size is the known object length; it bounds ranged reads and anchors
// io.SeekEnd. The context is captured for the adapter's lifetime because
// io.Reader carries no context — pass the context of the request the reader
// belongs to.
func NewRangeReadSeeker(ctx context.Context, size int64, open RangeOpener) *RangeReadSeeker {
	return &RangeReadSeeker{ctx: ctx, open: open, size: size}
}

// Read reads from the current position, opening a ranged request on demand.
// It returns io.EOF once the position reaches the object size, and
// io.ErrUnexpectedEOF when the opened body ends before the requested range.
func (r *RangeReadSeeker) Read(p []byte) (int, error) {
	if r.pos >= r.size {
		return 0, io.EOF
	}
	if r.body == nil {
		body, err := r.open(r.ctx, r.pos, r.size-r.pos)
		if err != nil {
			return 0, fmt.Errorf("open ranged read at offset %d: %w", r.pos, err)
		}
		r.body = body
	}
	n, err := r.body.Read(p)
	r.pos += int64(n)
	if errors.Is(err, io.EOF) {
		r.closeBody()
		if r.pos < r.size {
			return n, io.ErrUnexpectedEOF
		}
	}
	return n, err
}

// Seek moves the read position. Seeking away from the current position closes
// the open body so the next Read reopens at the new offset.
func (r *RangeReadSeeker) Seek(offset int64, whence int) (int64, error) {
	var pos int64
	switch whence {
	case io.SeekStart:
		pos = offset
	case io.SeekCurrent:
		pos = r.pos + offset
	case io.SeekEnd:
		pos = r.size + offset
	default:
		return 0, fmt.Errorf("%w: %d", ErrInvalidWhence, whence)
	}
	if pos < 0 {
		return 0, fmt.Errorf("%w: %d", ErrNegativePosition, pos)
	}
	if pos != r.pos {
		r.closeBody()
		r.pos = pos
	}
	return pos, nil
}

// Close releases the open body, if any. It is idempotent.
func (r *RangeReadSeeker) Close() error {
	if r.body == nil {
		return nil
	}
	body := r.body
	r.body = nil
	return body.Close()
}

func (r *RangeReadSeeker) closeBody() {
	if r.body != nil {
		_ = r.body.Close() //nolint:errcheck // best-effort release before reopening at a new offset
		r.body = nil
	}
}
