// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package etag_test

import (
	"crypto/sha1" //nolint:gosec // test-only: verifies a pluggable non-default hash.
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/encoding/etag"

	coreio "github.com/altessa-s/go-atlas/core/io"
)

func TestGeneratorDefaultSHA256(t *testing.T) {
	t.Parallel()

	const body = "hello world"
	sum := sha256.Sum256([]byte(body))
	want := etag.Strong(hex.EncodeToString(sum[:]))

	g := etag.NewGenerator()

	require.Equal(t, want, g.Hash([]byte(body)))
	require.Equal(t, want, g.HashString(body))

	got, err := g.HashReader(strings.NewReader(body))
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestGeneratorEmptyInput(t *testing.T) {
	t.Parallel()

	sum := sha256.Sum256(nil)
	want := etag.Strong(hex.EncodeToString(sum[:]))

	g := etag.NewGenerator()
	require.Equal(t, want, g.Hash(nil))
	require.Equal(t, want, g.HashString(""))
}

func TestGeneratorWithNewHash(t *testing.T) {
	t.Parallel()

	const body = "hello world"
	g := etag.NewGenerator(etag.WithNewHash(sha1.New))

	sum := sha1.Sum([]byte(body)) //nolint:gosec // test-only.
	require.Equal(t, etag.Strong(hex.EncodeToString(sum[:])), g.Hash([]byte(body)))

	// A different hash yields a different tag than the default.
	require.NotEqual(t, etag.NewGenerator().Hash([]byte(body)), g.Hash([]byte(body)))
}

func TestGeneratorHashReaderError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("boom")
	g := etag.NewGenerator()

	tag, err := g.HashReader(coreio.NewErrorReader(sentinel))
	require.ErrorIs(t, err, sentinel)
	require.True(t, tag.IsZero())
}
