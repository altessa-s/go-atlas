// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package spool_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/io/spool"
)

func TestWithTempDir(t *testing.T) {
	t.Parallel()

	for _, tee := range []bool{false, true} {
		dir := t.TempDir()
		opts := []spool.Option{spool.WithMemThreshold(4), spool.WithTempDir(dir)}
		if tee {
			opts = append(opts, spool.WithTee(io.Discard))
		}
		sp, err := spool.New(strings.NewReader("spills to disk"), opts...)
		require.NoError(t, err)

		files, err := filepath.Glob(filepath.Join(dir, "spool-*"))
		require.NoError(t, err)
		require.Len(t, files, 1, "the spill file lives in the configured directory (tee=%v)", tee)

		var got bytes.Buffer
		_, err = io.Copy(&got, sp)
		require.NoError(t, err)
		require.Equal(t, "spills to disk", got.String())
		require.NoError(t, sp.Close())

		_, err = os.Stat(files[0])
		require.ErrorIs(t, err, os.ErrNotExist, "Close removes the spill file")
	}
}

func TestWithTempDirMissing(t *testing.T) {
	t.Parallel()

	_, err := spool.New(strings.NewReader("spills to disk"),
		spool.WithMemThreshold(4), spool.WithTempDir(filepath.Join(t.TempDir(), "absent")))
	require.ErrorIs(t, err, os.ErrNotExist)
}
