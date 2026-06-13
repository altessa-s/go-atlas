// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spool_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/io/spool"
)

func isFileBacked(sp *spool.Spool) bool {
	_, ok := sp.ReadSeeker.(interface{ Name() string }) // *os.File exposes Name
	return ok
}

func TestNew_BackingStoreAndSize(t *testing.T) {
	t.Parallel()

	const body = "hello world, this is the spooled body"

	cases := []struct {
		name      string
		threshold int64
		wantFile  bool
	}{
		{name: "fits in memory", threshold: int64(len(body)), wantFile: false},
		{name: "spills to temp file", threshold: 8, wantFile: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sp, err := spool.New(strings.NewReader(body), spool.WithMemThreshold(tc.threshold))
			require.NoError(t, err)
			t.Cleanup(func() { _ = sp.Close() }) //nolint:errcheck // best-effort test cleanup

			require.Equal(t, tc.wantFile, isFileBacked(sp), "unexpected backing store")
			require.Equal(t, int64(len(body)), sp.Size())

			got, err := io.ReadAll(sp)
			require.NoError(t, err)
			require.Equal(t, body, string(got))
		})
	}
}

func TestNew_RewindIsRepeatable(t *testing.T) {
	t.Parallel()

	const body = "rewind me as many times as you like"

	// A tiny threshold forces the temp-file path, where a rewind is a real Seek.
	sp, err := spool.New(strings.NewReader(body), spool.WithMemThreshold(4))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sp.Close() }) //nolint:errcheck // best-effort test cleanup

	for range 3 {
		_, err := sp.Seek(0, io.SeekStart)
		require.NoError(t, err)
		got, err := io.ReadAll(sp)
		require.NoError(t, err)
		require.Equal(t, body, string(got))
	}
}

func TestNew_TeeObservesFullContentOnce(t *testing.T) {
	t.Parallel()

	body := bytes.Repeat([]byte("payload-"), 4096)
	want := sha256.Sum256(body)

	cases := []struct {
		name      string
		threshold int64
	}{
		{name: "memory path", threshold: int64(len(body))},
		{name: "spill path", threshold: 16},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			h := sha256.New()
			sp, err := spool.New(bytes.NewReader(body),
				spool.WithMemThreshold(tc.threshold),
				spool.WithTee(h),
			)
			require.NoError(t, err)
			t.Cleanup(func() { _ = sp.Close() }) //nolint:errcheck // best-effort test cleanup
			require.Equal(t, hex.EncodeToString(want[:]), hex.EncodeToString(h.Sum(nil)),
				"tee must observe the full content exactly once")
			require.Equal(t, int64(len(body)), sp.Size())
		})
	}
}

func TestNew_MaxBytes(t *testing.T) {
	t.Parallel()

	body := strings.Repeat("x", 100)

	t.Run("exactly at cap is allowed", func(t *testing.T) {
		t.Parallel()
		sp, err := spool.New(strings.NewReader(body), spool.WithMaxBytes(100))
		require.NoError(t, err)
		t.Cleanup(func() { _ = sp.Close() }) //nolint:errcheck // best-effort test cleanup
		require.Equal(t, int64(100), sp.Size())
	})

	t.Run("over cap in memory path", func(t *testing.T) {
		t.Parallel()
		_, err := spool.New(strings.NewReader(body), spool.WithMaxBytes(99))
		require.ErrorIs(t, err, spool.ErrTooLarge)
	})

	t.Run("over cap in spill path", func(t *testing.T) {
		t.Parallel()
		_, err := spool.New(strings.NewReader(body),
			spool.WithMemThreshold(8),
			spool.WithMaxBytes(50),
		)
		require.ErrorIs(t, err, spool.ErrTooLarge)
	})
}

func TestNew_CloseRemovesTempFileAndIsIdempotent(t *testing.T) {
	t.Parallel()

	sp, err := spool.New(bytes.NewReader(make([]byte, 64)), spool.WithMemThreshold(8))
	require.NoError(t, err)

	f, ok := sp.ReadSeeker.(interface{ Name() string })
	require.True(t, ok, "expected a temp-file-backed spool")
	name := f.Name()

	require.NoError(t, sp.Close())
	require.NoFileExists(t, name, "Close must remove the temp file")
	require.NoError(t, sp.Close(), "Close must be idempotent")
}

func TestNew_InMemoryCloseIsNoop(t *testing.T) {
	t.Parallel()

	sp, err := spool.New(strings.NewReader("small"))
	require.NoError(t, err)
	require.False(t, isFileBacked(sp))
	require.NoError(t, sp.Close())
	require.NoError(t, sp.Close())
}
