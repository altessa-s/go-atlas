// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package checksum_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa/internal/checksum"

	corehash "github.com/altessa-s/go-atlas-core/encoding/hash"
)

func TestManifest(t *testing.T) {
	t.Parallel()
	mismatch, unexpected, missing := errors.New("mismatch"), errors.New("unexpected"), errors.New("missing")
	entries := map[string]string{"policy.rego": corehash.SHA256HexBytes([]byte("policy"))}
	m := checksum.New(entries)
	clear(entries)
	require.NoError(t, m.Verify("policy.rego", []byte("policy"), mismatch, unexpected))
	require.ErrorIs(t, m.Verify("policy.rego", []byte("modified"), mismatch, unexpected), mismatch)
	require.ErrorIs(t, m.Verify("other.rego", nil, mismatch, unexpected), unexpected)
	require.ErrorIs(t, m.Covered(nil, missing), missing)
	require.NoError(t, m.Covered(map[string]string{"policy.rego": "path"}, missing))
	require.NoError(t, checksum.New(nil).Verify("any", nil, mismatch, unexpected))
	require.ErrorIs(t, checksum.New(map[string]string{}).Verify("any", nil, mismatch, unexpected), unexpected)
}

func TestDecodeRelease(t *testing.T) {
	t.Parallel()

	const digest = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	root, entries, err := checksum.DecodeRelease[string]([]byte(`{"nested/policy.rego":"` + digest + `"}`))
	require.NoError(t, err)
	require.Empty(t, root)
	require.Equal(t, map[string]string{"nested/policy.rego": digest}, entries)

	for _, tc := range []struct {
		name    string
		content string
	}{
		{name: "null", content: `null`},
		{name: "escape", content: `{"../policy.rego":"abc"}`},
		{name: "duplicate", content: `{"a.rego":"abc","a.rego":"def"}`},
		{name: "trailing data", content: `{}{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := checksum.DecodeRelease[string]([]byte(tc.content))
			require.Error(t, err)
		})
	}
}
