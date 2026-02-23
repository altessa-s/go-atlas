// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ocsp

import (
	"testing"
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
		if err != nil {
			t.Fatalf("compressData() error = %v", err)
		}

		decompressed, err := decompressData(compressed)
		if err != nil {
			t.Fatalf("decompressData() error = %v", err)
		}

		if len(decompressed) != len(data) {
			t.Errorf("roundtrip length mismatch: got %d, want %d", len(decompressed), len(data))
		}
	})
}
