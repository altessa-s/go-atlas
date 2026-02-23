// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package io

import (
	"errors"
	"fmt"
	"io"
	"sync"
)

// ErrReadLimitExceeded is a sentinel error returned by [LimitedReadCloser.Read] when the
// cumulative number of bytes read exceeds the configured limit. Callers can test for this
// condition with [errors.Is].
var ErrReadLimitExceeded = errors.New("read limit exceeded")

// ErrorReader is an [io.Reader] implementation that returns zero bytes read and
// the configured [ErrorReader.Err] on every call to [ErrorReader.Read]. It is useful
// in tests and error-path simulations where a reader must fail deterministically.
// Create instances with [NewErrorReader].
type ErrorReader struct {
	// Err is the error returned by every call to [ErrorReader.Read].
	Err error
}

// Read always returns 0 bytes read and the configured [ErrorReader.Err]. The
// destination slice is never modified.
func (e *ErrorReader) Read(_ []byte) (int, error) {
	return 0, e.Err
}

// NewErrorReader creates an [ErrorReader] whose [ErrorReader.Read] method always
// returns (0, err). The supplied error is stored as-is and not wrapped.
func NewErrorReader(err error) *ErrorReader {
	return &ErrorReader{Err: err}
}

// LimitedReadCloser wraps an [io.ReadCloser] and enforces a maximum number of bytes
// that may be read. If a Read call causes the cumulative byte count to exceed the
// configured limit, Read returns an error wrapping [ErrReadLimitExceeded]. The
// underlying reader's Close method is preserved and delegated to by
// [LimitedReadCloser.Close].
//
// LimitedReadCloser is safe for concurrent reads; an internal mutex protects the
// running byte counter. Create instances with [NewLimitedReadCloser].
type LimitedReadCloser struct {
	reader io.Reader
	closer io.Closer
	limit  int64
	read   int64
	mu     sync.Mutex
}

// NewLimitedReadCloser creates a [LimitedReadCloser] that allows at most limit bytes
// to be read from rc. Internally, the underlying reader is wrapped with an
// [io.LimitReader] set to limit+1 so that an over-read can be detected and reported
// as [ErrReadLimitExceeded] rather than a silent truncation at the boundary.
func NewLimitedReadCloser(rc io.ReadCloser, limit int64) *LimitedReadCloser {
	return &LimitedReadCloser{
		reader: io.LimitReader(rc, limit+1),
		closer: rc,
		limit:  limit,
	}
}

// Read reads up to len(p) bytes from the underlying reader into p. It returns the
// number of bytes read and any error encountered. If the cumulative bytes read exceed
// the configured limit, Read returns an error wrapping [ErrReadLimitExceeded]. The
// byte counter is protected by a mutex, making concurrent Read calls safe.
func (l *LimitedReadCloser) Read(p []byte) (n int, err error) {
	n, err = l.reader.Read(p)

	l.mu.Lock()
	l.read += int64(n)
	totalRead := l.read
	l.mu.Unlock()

	// Check if we've exceeded the limit
	if totalRead > l.limit {
		return n, fmt.Errorf("%w: %d bytes read, limit is %d", ErrReadLimitExceeded, totalRead, l.limit)
	}

	return n, err
}

// Close delegates to the underlying [io.ReadCloser]'s Close method. If the underlying
// closer is nil, Close returns nil.
func (l *LimitedReadCloser) Close() error {
	if l.closer != nil {
		return l.closer.Close()
	}
	return nil
}
