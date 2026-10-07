// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package plugins

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkReadAndHashFile(b *testing.B) {
	dir := b.TempDir()
	path := filepath.Join(dir, "bench.so")
	// 1 MB file — representative of a small plugin.
	require.NoError(b, os.WriteFile(path, make([]byte, 1<<20), 0o644))

	b.ResetTimer()
	for b.Loop() {
		_, _, _ = readAndHashFile(path)
	}
}

func BenchmarkIsQuarantined_Miss(b *testing.B) {
	mgr := NewManager(WithSignatureDisabled())
	mgr.quarantine["other.so"] = "somehash"

	b.ResetTimer()
	for b.Loop() {
		mgr.isQuarantined("target.so", "anyhash")
	}
}

func BenchmarkIsQuarantined_Hit(b *testing.B) {
	mgr := NewManager(WithSignatureDisabled())
	mgr.quarantine["target.so"] = "matchhash"

	b.ResetTimer()
	for b.Loop() {
		mgr.isQuarantined("target.so", "matchhash")
	}
}
