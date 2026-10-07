// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package io_test

import (
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	coreio "github.com/altessa-s/go-atlas/core/io"
)

func TestErrorReader(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("expected error")
	reader := coreio.NewErrorReader(expectedErr)

	buf := make([]byte, 10)
	n, err := reader.Read(buf)

	require.Equal(t, 0, n, "Read bytes")
	require.Equal(t, expectedErr, err, "Read error")
}

func TestLimitedReadCloser(t *testing.T) {
	t.Parallel()

	t.Run("UnderLimit", func(t *testing.T) {
		t.Parallel()
		baseReader := io.NopCloser(strings.NewReader("hello"))
		lrc := coreio.NewLimitedReadCloser(baseReader, 10)

		buf, err := io.ReadAll(lrc)
		require.NoError(t, err)
		require.Equal(t, "hello", string(buf))
		require.NoError(t, lrc.Close())
	})

	t.Run("AtLimit", func(t *testing.T) {
		t.Parallel()
		baseReader := io.NopCloser(strings.NewReader("hello"))
		lrc := coreio.NewLimitedReadCloser(baseReader, 5)

		buf, err := io.ReadAll(lrc)
		require.NoError(t, err)
		require.Equal(t, "hello", string(buf))
	})

	t.Run("OverLimit", func(t *testing.T) {
		t.Parallel()
		baseReader := io.NopCloser(strings.NewReader("hello world"))
		lrc := coreio.NewLimitedReadCloser(baseReader, 5)

		_, err := io.ReadAll(lrc)
		require.ErrorIs(t, err, coreio.ErrReadLimitExceeded)
	})

	t.Run("CloseCallsUnderlying", func(t *testing.T) {
		t.Parallel()
		mrc := &testhelpers.MockReadCloser{Reader: strings.NewReader("test")}
		lrc := coreio.NewLimitedReadCloser(mrc, 100)
		lrc.Close()
		require.True(t, mrc.Closed, "Close() did not call underlying Close()")
	})
}

func TestErrorReader_CustomError(t *testing.T) {
	t.Parallel()

	customErr := errors.New("custom read error")
	reader := coreio.NewErrorReader(customErr)

	buf := make([]byte, 5)
	n, err := reader.Read(buf)
	require.Equal(t, 0, n, "Read bytes")
	require.ErrorIs(t, err, customErr)
}

func TestLimitedReadCloser_ZeroLimit(t *testing.T) {
	t.Parallel()

	baseReader := io.NopCloser(strings.NewReader("data"))
	lrc := coreio.NewLimitedReadCloser(baseReader, 0)

	_, err := io.ReadAll(lrc)
	require.ErrorIs(t, err, coreio.ErrReadLimitExceeded)
}

func TestLimitedReadCloser_ExactlyOneByte(t *testing.T) {
	t.Parallel()

	baseReader := io.NopCloser(strings.NewReader("x"))
	lrc := coreio.NewLimitedReadCloser(baseReader, 1)

	buf, err := io.ReadAll(lrc)
	require.NoError(t, err)
	require.Equal(t, "x", string(buf))
}

// endlessReader is a goroutine-safe source of zero bytes that counts what it
// served.
type endlessReader struct {
	mu     sync.Mutex
	served int64
}

func (r *endlessReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.served += int64(len(p))
	clear(p)
	return len(p), nil
}

func (*endlessReader) Close() error { return nil }

// Concurrent reads must not race on the wrapped limit reader nor pull more than
// limit+1 bytes from the source.
func TestLimitedReadCloser_ConcurrentReads(t *testing.T) {
	t.Parallel()

	const limit = 1000
	src := &endlessReader{}
	lrc := coreio.NewLimitedReadCloser(src, limit)

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			buf := make([]byte, 7)
			for {
				if _, err := lrc.Read(buf); err != nil {
					return
				}
			}
		})
	}
	wg.Wait()

	src.mu.Lock()
	defer src.mu.Unlock()
	require.LessOrEqual(t, src.served, int64(limit+1))
}

func TestBufferPool(t *testing.T) {
	t.Parallel()

	buf := coreio.GetBuffer()
	require.NotNil(t, buf)
	buf.WriteString("test")
	coreio.PutBuffer(buf)

	buf2 := coreio.GetBuffer()
	require.Equal(t, 0, buf2.Len(), "expected reset buffer from pool")
	coreio.PutBuffer(buf2)
}
