// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package ocsp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDecompressData_RejectsOversizedPayload verifies that the cache
// decompression path refuses to materialize more than maxOCSPResponseSize
// bytes even when the compressed input is small (zip-bomb guard).
func TestDecompressData_RejectsOversizedPayload(t *testing.T) {
	t.Parallel()

	compressed, err := compressData(make([]byte, maxOCSPResponseSize+1))
	require.NoError(t, err)

	_, err = decompressData(compressed)
	require.Error(t, err)
}
