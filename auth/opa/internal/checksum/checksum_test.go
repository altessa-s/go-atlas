// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package checksum_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/opa/internal/checksum"

	corehash "github.com/altessa-s/go-atlas/core/encoding/hash"
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
