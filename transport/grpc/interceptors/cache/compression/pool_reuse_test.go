// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package compression

import (
	"bytes"
	"compress/gzip"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReaderRetainsInflaterAndReleasesInput(t *testing.T) {
	t.Parallel()
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	_, err := w.Write([]byte("payload"))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	r := &gzipReader{}
	r.input.Reset(compressed.Bytes())
	r.reader, err = gzip.NewReader(&r.input)
	require.NoError(t, err)
	_, err = io.Copy(io.Discard, r.reader)
	require.NoError(t, err)
	reader := r.reader
	require.True(t, resetGzipReader(r))
	require.Same(t, reader, r.reader)
	require.Zero(t, r.input.Size())
}
