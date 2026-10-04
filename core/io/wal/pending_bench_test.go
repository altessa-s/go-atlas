// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package wal_test

import (
	"github.com/altessa-s/go-atlas/core/io/wal"
	"github.com/stretchr/testify/require"
	"testing"
)

func BenchmarkReadPending(b *testing.B) {
	w, _, err := wal.Open(b.TempDir())
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, w.Close()) })
	for range 100 {
		_, err = w.Append(make([]byte, 64))
		require.NoError(b, err)
	}
	b.ReportAllocs()
	for b.Loop() {
		records, err := w.ReadPending(100, nil)
		require.NoError(b, err)
		require.Len(b, records, 100)
	}
}
