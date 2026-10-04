// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package compression_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache/compression"
)

func TestHighlyCompressiblePayloadRoundtrip(t *testing.T) {
	t.Parallel()
	for _, size := range []int{32 * 1024, 2 * 1024 * 1024} {
		for _, level := range []int{1, 6, 9} {
			t.Run(fmt.Sprintf("size_%d_level_%d", size, level), func(t *testing.T) {
				t.Parallel()
				c := compression.NewCompressor(1, 0, level)
				input := bytes.Repeat([]byte("a"), size)
				encoded, err := c.Compress(t.Context(), input)
				require.NoError(t, err)
				decoded, err := c.Decompress(t.Context(), encoded)
				require.NoError(t, err)
				require.Equal(t, input, decoded)
			})
		}
	}
}
