// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filesystem_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa/sources/filesystem"
)

func BenchmarkSource_Fetch(b *testing.B) {
	dir := b.TempDir()
	policyContent := []byte("package test\ndefault allow = false")
	policyFile := filepath.Join(dir, "test.rego")
	require.NoError(b, os.WriteFile(policyFile, policyContent, 0o600))

	source, err := filesystem.New(dir, writeManifest(b, dir, "test.rego"))
	require.NoError(b, err)
	defer source.Close()

	ctx := b.Context()

	b.ResetTimer()
	for b.Loop() {
		_, err := source.Fetch(ctx)
		require.NoError(b, err)
	}
}
