// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ocsp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func FuzzCompressDecompressRoundtrip(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("hello world"))
	f.Add([]byte("OCSP response data"))
	f.Add(make([]byte, 1024))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}

		compressed, err := compressData(data)
		if !assert.NoError(t, err) {
			return
		}

		decompressed, err := decompressData(compressed)
		if !assert.NoError(t, err) {
			return
		}

		assert.Len(t, decompressed, len(data))
	})
}
