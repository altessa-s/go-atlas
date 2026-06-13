// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package io_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	coreio "github.com/altessa-s/go-atlas/core/io"
)

// rangeCall records one RangeOpener invocation.
type rangeCall struct {
	offset, length int64
}

// rangeSource serves a RangeOpener over an in-memory byte slice and records
// every requested range. truncateTo, when positive, caps the returned body
// length to simulate a source returning fewer bytes than the requested range.
type rangeSource struct {
	mu         sync.Mutex
	data       []byte
	calls      []rangeCall
	openErr    error
	truncateTo int
}

func (s *rangeSource) open(_ context.Context, offset, length int64) (io.ReadCloser, error) {
	if s.openErr != nil {
		return nil, s.openErr
	}
	s.mu.Lock()
	s.calls = append(s.calls, rangeCall{offset: offset, length: length})
	s.mu.Unlock()
	end := offset + length
	if end > int64(len(s.data)) {
		end = int64(len(s.data))
	}
	body := s.data[offset:end]
	if s.truncateTo > 0 && len(body) > s.truncateTo {
		body = body[:s.truncateTo]
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

const rangeTestObject = "hello world"

func newTestRangeReadSeeker(t *testing.T, src *rangeSource) *coreio.RangeReadSeeker {
	t.Helper()
	return coreio.NewRangeReadSeeker(t.Context(), int64(len(src.data)), src.open)
}

func TestRangeReadSeeker_SequentialReadWholeObject(t *testing.T) {
	t.Parallel()

	src := &rangeSource{data: []byte(rangeTestObject)}
	rs := newTestRangeReadSeeker(t, src)
	defer rs.Close() //nolint:errcheck // released again explicitly below

	got, err := io.ReadAll(rs)
	require.NoError(t, err)
	require.Equal(t, rangeTestObject, string(got))
	require.Equal(t, []rangeCall{{offset: 0, length: int64(len(rangeTestObject))}}, src.calls,
		"a sequential read must open exactly one ranged request from position 0")
	require.NoError(t, rs.Close())
}

func TestRangeReadSeeker_SeekForwardOpensNewRange(t *testing.T) {
	t.Parallel()

	src := &rangeSource{data: []byte(rangeTestObject)}
	rs := newTestRangeReadSeeker(t, src)
	defer rs.Close() //nolint:errcheck // best-effort test cleanup

	head := make([]byte, 5)
	_, err := io.ReadFull(rs, head)
	require.NoError(t, err)
	require.Equal(t, "hello", string(head))

	pos, err := rs.Seek(6, io.SeekStart)
	require.NoError(t, err)
	require.Equal(t, int64(6), pos)

	rest, err := io.ReadAll(rs)
	require.NoError(t, err)
	require.Equal(t, "world", string(rest))

	require.Len(t, src.calls, 2)
	require.Equal(t, int64(6), src.calls[1].offset,
		"the read after a forward seek must open a ranged request at the new position")
}

func TestRangeReadSeeker_SeekWhence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		prepare func(t *testing.T, rs *coreio.RangeReadSeeker)
		offset  int64
		whence  int
		wantPos int64
		want    string
	}{
		{
			name:    "seek end",
			offset:  -5,
			whence:  io.SeekEnd,
			wantPos: 6,
			want:    "world",
		},
		{
			name: "seek current",
			prepare: func(t *testing.T, rs *coreio.RangeReadSeeker) {
				t.Helper()
				_, err := rs.Seek(2, io.SeekStart)
				require.NoError(t, err)
			},
			offset:  4,
			whence:  io.SeekCurrent,
			wantPos: 6,
			want:    "world",
		},
		{
			name:    "seek past end yields EOF on read",
			offset:  2,
			whence:  io.SeekEnd,
			wantPos: int64(len(rangeTestObject)) + 2,
			want:    "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := &rangeSource{data: []byte(rangeTestObject)}
			rs := newTestRangeReadSeeker(t, src)
			defer rs.Close() //nolint:errcheck // best-effort test cleanup

			if tc.prepare != nil {
				tc.prepare(t, rs)
			}
			pos, err := rs.Seek(tc.offset, tc.whence)
			require.NoError(t, err)
			require.Equal(t, tc.wantPos, pos)

			got, err := io.ReadAll(rs)
			require.NoError(t, err)
			require.Equal(t, tc.want, string(got))
		})
	}
}

func TestRangeReadSeeker_SeekErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		offset  int64
		whence  int
		wantErr error
	}{
		{name: "negative position", offset: -1, whence: io.SeekStart, wantErr: coreio.ErrNegativePosition},
		{name: "negative position via current", offset: -10, whence: io.SeekCurrent, wantErr: coreio.ErrNegativePosition},
		{name: "invalid whence", offset: 0, whence: 42, wantErr: coreio.ErrInvalidWhence},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := &rangeSource{data: []byte(rangeTestObject)}
			rs := newTestRangeReadSeeker(t, src)
			defer rs.Close() //nolint:errcheck // best-effort test cleanup

			_, err := rs.Seek(tc.offset, tc.whence)
			require.ErrorIs(t, err, tc.wantErr)
			require.Empty(t, src.calls, "a failed seek must not open a ranged read")
		})
	}
}

func TestRangeReadSeeker_ReadAfterEOF(t *testing.T) {
	t.Parallel()

	src := &rangeSource{data: []byte(rangeTestObject)}
	rs := newTestRangeReadSeeker(t, src)
	defer rs.Close() //nolint:errcheck // best-effort test cleanup

	_, err := io.ReadAll(rs)
	require.NoError(t, err)

	n, err := rs.Read(make([]byte, 1))
	require.Zero(t, n)
	require.ErrorIs(t, err, io.EOF)
}

func TestRangeReadSeeker_ShortBodyIsUnexpectedEOF(t *testing.T) {
	t.Parallel()

	src := &rangeSource{data: []byte(rangeTestObject), truncateTo: 5}
	rs := newTestRangeReadSeeker(t, src)
	defer rs.Close() //nolint:errcheck // best-effort test cleanup

	_, err := io.ReadAll(rs)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF,
		"a body shorter than the requested range must surface io.ErrUnexpectedEOF")
}

func TestRangeReadSeeker_OpenErrorIsWrapped(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("source down")
	src := &rangeSource{data: []byte(rangeTestObject), openErr: sentinel}
	rs := newTestRangeReadSeeker(t, src)
	defer rs.Close() //nolint:errcheck // best-effort test cleanup

	_, err := rs.Read(make([]byte, 1))
	require.ErrorIs(t, err, sentinel)
}

func TestRangeReadSeeker_CloseIsIdempotent(t *testing.T) {
	t.Parallel()

	src := &rangeSource{data: []byte(rangeTestObject)}
	rs := newTestRangeReadSeeker(t, src)

	_, err := rs.Read(make([]byte, 3)) // open a body
	require.NoError(t, err)

	require.NoError(t, rs.Close())
	require.NoError(t, rs.Close(), "Close must be idempotent")
}
