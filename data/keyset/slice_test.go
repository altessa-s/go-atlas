// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keyset_test

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/keyset"
)

var sliceBindings = keyset.NewBindings("list", "name:1")

func TestPageSize(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		requested    int64
		def, maxSize int
		want         int
	}{
		"negative is default":  {requested: -1, def: 50, maxSize: 200, want: 50},
		"zero is default":      {requested: 0, def: 50, maxSize: 200, want: 50},
		"within range":         {requested: 120, def: 50, maxSize: 200, want: 120},
		"one":                  {requested: 1, def: 50, maxSize: 200, want: 1},
		"at max":               {requested: 200, def: 50, maxSize: 200, want: 200},
		"clamped":              {requested: 201, def: 50, maxSize: 200, want: 200},
		"huge clamped":         {requested: math.MaxInt64, def: 50, maxSize: 200, want: 200},
		"default above max":    {requested: 0, def: 500, maxSize: 200, want: 200},
		"default not positive": {requested: 0, def: 0, maxSize: 200, want: 1},
		"max not positive":     {requested: 10, def: 50, maxSize: 0, want: 1},
		"negative max":         {requested: 0, def: 50, maxSize: -5, want: 1},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, keyset.PageSize(tc.requested, tc.def, tc.maxSize))
		})
	}

	assert.Equal(t, 10, keyset.PageSize(int32(-5), 10, 100))
	assert.Equal(t, 100, keyset.PageSize(uint64(math.MaxUint64), 10, 100))
	assert.Equal(t, 7, keyset.PageSize(uint8(7), 10, 100))
}

type item struct{ name string }

func names(items []item) string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.name
	}
	return strings.Join(out, ",")
}

func keysetPager(codec *keyset.Codec) keyset.SlicePager[item] {
	return keyset.NewSlicePager(keyset.NewTyped[string](codec), func(it item) string { return it.name }, strings.Compare)
}

func TestSlicePager_WalksAllPages(t *testing.T) {
	t.Parallel()

	items := []item{{"a"}, {"b"}, {"c"}, {"d"}, {"e"}}
	codec := newCodec(t)
	for name, pager := range map[string]keyset.SlicePager[item]{
		"keyset": keysetPager(codec),
		"offset": keyset.NewSliceOffsetPager[item](keyset.NewOffset(codec)),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var got []string
			token := ""
			for range len(items) {
				page, err := pager.Page(items, 2, token, sliceBindings)
				require.NoError(t, err)
				got = append(got, names(page.Items))
				if token = page.Next; token == "" {
					break
				}
			}
			assert.Equal(t, []string{"a,b", "c,d", "e"}, got)
		})
	}
}

// TestSlicePager_KeysetSurvivesChanges removes and adds items between pages:
// a keyset position neither repeats nor skips the items that stay.
func TestSlicePager_KeysetSurvivesChanges(t *testing.T) {
	t.Parallel()

	pager := keysetPager(newCodec(t))
	page, err := pager.Page([]item{{"a"}, {"b"}, {"c"}, {"d"}}, 2, "", sliceBindings)
	require.NoError(t, err)
	require.Equal(t, "a,b", names(page.Items))

	page, err = pager.Page([]item{{"a"}, {"bb"}, {"c"}, {"d"}}, 2, page.Next, sliceBindings)
	require.NoError(t, err)
	assert.Equal(t, "bb,c", names(page.Items), "an item added after the position is served")
	assert.NotEmpty(t, page.Next)

	page, err = pager.Page([]item{{"a"}}, 2, page.Next, sliceBindings)
	require.NoError(t, err)
	assert.Empty(t, page.Items, "every remaining item lies before the position")
	assert.Empty(t, page.Next)
}

func TestSlicePager_OffsetBeyondEnd(t *testing.T) {
	t.Parallel()

	codec := newCodec(t)
	pager := keyset.NewSliceOffsetPager[item](keyset.NewOffset(codec))
	token, err := keyset.NewOffset(codec).Issue(4, sliceBindings)
	require.NoError(t, err)

	page, err := pager.Page([]item{{"a"}, {"b"}, {"c"}, {"d"}}, 2, token, sliceBindings)
	require.NoError(t, err)
	assert.Empty(t, page.Items, "an offset at the end is the empty last page")
	assert.Empty(t, page.Next)

	_, err = pager.Page([]item{{"a"}, {"b"}, {"c"}}, 2, token, sliceBindings)
	require.ErrorIs(t, err, keyset.ErrInvalidToken, "the catalog shrank below the offset")
	assert.Equal(t, keyset.FailureMalformed, keyset.Classify(err))
}

func TestSlicePager_TokenFailures(t *testing.T) {
	t.Parallel()

	codec := newCodec(t)
	pager := keysetPager(codec)
	items := []item{{"a"}, {"b"}, {"c"}}
	page, err := pager.Page(items, 1, "", sliceBindings)
	require.NoError(t, err)

	_, err = pager.Page(items, 1, page.Next, keyset.NewBindings("other", "name:1"))
	require.ErrorIs(t, err, keyset.ErrSubjectMismatch)
	_, err = pager.Page(items, 1, "garbage!", sliceBindings)
	require.ErrorIs(t, err, keyset.ErrInvalidToken)
	_, err = keysetPager(nil).Page(items, 1, page.Next, sliceBindings)
	require.ErrorIs(t, err, keyset.ErrCodecRequired)
}

func TestSlicePager_WithoutCodecServesOnePage(t *testing.T) {
	t.Parallel()

	page, err := keysetPager(nil).Page([]item{{"a"}, {"b"}, {"c"}}, 2, "", sliceBindings)
	require.NoError(t, err)
	assert.Equal(t, "a,b", names(page.Items))
	assert.Empty(t, page.Next)
}

func TestSlicePager_SizeBelowOne(t *testing.T) {
	t.Parallel()

	page, err := keysetPager(newCodec(t)).Page([]item{{"a"}, {"b"}}, 0, "", sliceBindings)
	require.NoError(t, err)
	assert.Equal(t, "a", names(page.Items))
	assert.NotEmpty(t, page.Next)

	page, err = keysetPager(newCodec(t)).Page(nil, 5, "", sliceBindings)
	require.NoError(t, err)
	assert.Empty(t, page.Items)
	assert.Empty(t, page.Next)
}
