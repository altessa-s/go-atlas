// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// TestFromPaths_BacktickQuotedKeys locks the AIP-161 parser semantics: a
// backtick-quoted segment is one path component, even if it contains dots
// or other characters that would normally act as a separator. ToPaths
// round-trips back to the same quoted form.
func TestFromPaths_BacktickQuotedKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		paths     []string
		wantPaths []string
	}{
		{
			name:      "simple backtick segment",
			paths:     []string{"reviews.`John Smith`"},
			wantPaths: []string{"reviews.John Smith"},
		},
		{
			name:      "backtick segment with dot",
			paths:     []string{"metadata.`google.com/project`"},
			wantPaths: []string{"metadata.`google.com/project`"},
		},
		{
			name:      "backtick segment with nested field",
			paths:     []string{"reviews.`John Smith`.score"},
			wantPaths: []string{"reviews.John Smith.score"},
		},
		{
			name:      "backtick segment with dot and nested field",
			paths:     []string{"reviews.`a.b`.score"},
			wantPaths: []string{"reviews.`a.b`.score"},
		},
		{
			name:      "leading backtick segment",
			paths:     []string{"`top.level`.child"},
			wantPaths: []string{"`top.level`.child"},
		},
		{
			name: "multiple keys at same level",
			paths: []string{
				"reviews.`a.b`",
				"reviews.`c.d`",
			},
			wantPaths: []string{
				"reviews.`a.b`",
				"reviews.`c.d`",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mask := fieldmask.FromPaths(tc.paths...)
			got := mask.ToPaths()
			require.ElementsMatch(t, tc.wantPaths, got)
		})
	}
}

// TestContains_BacktickQuotedKeys ensures the Contains lookup splits the
// query path with the same backtick-aware rules used by FromPaths.
func TestContains_BacktickQuotedKeys(t *testing.T) {
	t.Parallel()

	mask := fieldmask.FromPaths("reviews.`John Smith`.score", "metadata.`google.com/project`")

	require.True(t, mask.Contains("reviews.`John Smith`.score"))
	require.True(t, mask.Contains("reviews.`John Smith`"))
	require.True(t, mask.Contains("metadata.`google.com/project`"))
	require.False(t, mask.Contains("reviews.`John Smith`.missing"))
	require.False(t, mask.Contains("reviews.John"))
}

// TestValidate_BacktickQuotedMapKey checks that validatePath accepts a
// backtick-quoted map key as a single segment against the descriptor —
// the key is opaque to the schema (any string is a valid map key).
func TestValidate_BacktickQuotedMapKey(t *testing.T) {
	t.Parallel()

	// labels is map<string, Profile>; quoted key may contain a dot.
	mask := fieldmask.FromPaths("labels.`group.admin`.display_name")
	require.NoError(t, mask.Validate(&pb.Resource{}))

	// Quoted segment is one segment, so nested access into a Profile
	// field still resolves through the descriptor.
	mask = fieldmask.FromPaths("labels.`group.admin`.id")
	require.NoError(t, mask.Validate(&pb.Resource{}))

	// Unknown nested field still triggers a ValidationError.
	mask = fieldmask.FromPaths("labels.`group.admin`.no_such_field")
	require.Error(t, mask.Validate(&pb.Resource{}))
}

// TestFilter_BacktickQuotedMapKey covers the iterateFilter map branch:
// the lookup of MapKey.String() against the per-key sub-mask must match
// even when the key contains a dot (so was passed in via backticks).
func TestFilter_BacktickQuotedMapKey(t *testing.T) {
	t.Parallel()

	res := &pb.Resource{
		Labels: map[string]*pb.Profile{
			"group.admin": {Id: "p1", DisplayName: "kept"},
			"group.guest": {Id: "p2", DisplayName: "dropped"},
		},
	}

	// Keep only the "group.admin" entry; inside it keep display_name.
	mask := fieldmask.FromPaths("labels.`group.admin`.display_name")
	mask.Filter(res)

	require.Len(t, res.Labels, 1)
	kept, ok := res.Labels["group.admin"]
	require.True(t, ok)
	require.Equal(t, "kept", kept.GetDisplayName())
	// id was not in the mask, so it must be cleared.
	require.Empty(t, kept.GetId())
}

// TestPrune_BacktickQuotedMapKey covers iteratePrune for map branches:
// the mask removes only the entry addressed by the quoted key, leaving
// siblings untouched.
func TestPrune_BacktickQuotedMapKey(t *testing.T) {
	t.Parallel()

	res := &pb.Resource{
		Labels: map[string]*pb.Profile{
			"group.admin": {Id: "p1", DisplayName: "first"},
			"group.guest": {Id: "p2", DisplayName: "second"},
		},
	}

	mask := fieldmask.FromPaths("labels.`group.admin`")
	mask.Prune(res)

	require.Len(t, res.Labels, 1)
	_, removed := res.Labels["group.admin"]
	require.False(t, removed)
	survived, ok := res.Labels["group.guest"]
	require.True(t, ok)
	require.Equal(t, "second", survived.GetDisplayName())
}
