// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package compression

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func FuzzGzipCompressor_CompressDecompress(f *testing.F) {
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add(make([]byte, 2048))

	c := NewCompressor(10, 0, 6)
	ctx := f.Context()

	f.Fuzz(func(t *testing.T, data []byte) {
		compressed, err := c.Compress(ctx, data)
		if err != nil {
			return
		}
		decompressed, err := c.Decompress(ctx, compressed)
		assert.NoError(t, err)
		assert.Equal(t, string(data), string(decompressed))
	})
}
