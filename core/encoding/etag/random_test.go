// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package etag_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/encoding/etag"
)

func TestRandom(t *testing.T) {
	t.Parallel()

	tag := etag.Random()
	require.True(t, tag.IsWeak(), "Random must be weak per RFC 7232")
	require.NotEmpty(t, tag.Value())
	require.Equal(t, `W/"`+tag.Value()+`"`, tag.String())

	// A generated tag is a valid entity-tag and round-trips through Parse.
	got, err := etag.Parse(tag.String())
	require.NoError(t, err)
	require.Equal(t, tag, got)

	// Each call yields a distinct, non-matching tag.
	other := etag.Random()
	require.NotEqual(t, tag, other)
	require.False(t, tag.WeakMatch(other))
}
