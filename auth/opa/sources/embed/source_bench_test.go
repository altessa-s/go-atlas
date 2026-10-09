// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package embed_test

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"

	embedsource "github.com/altessa-s/go-atlas/auth/opa/sources/embed"
	corehash "github.com/altessa-s/go-atlas/core/encoding/hash"
)

func BenchmarkFetchWithChecksum(b *testing.B) {
	content := []byte("package example\ndefault allow := false\n")
	source, err := embedsource.New(fstest.MapFS{"policy.rego": &fstest.MapFile{Data: content}}, ".",
		embedsource.WithChecksums(map[string]string{"policy.rego": corehash.SHA256HexBytes(content)}))
	require.NoError(b, err)
	b.Cleanup(func() { require.NoError(b, source.Close()) })
	b.ReportAllocs()
	for b.Loop() {
		_, err := source.Fetch(b.Context())
		require.NoError(b, err)
	}
}
