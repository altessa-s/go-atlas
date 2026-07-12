// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// TestFromPaths_Wildcard_RoundTrip locks the parser semantics for the
// AIP-161 wildcard segment: "*" is a single path component, stored as a
// regular key in the FieldMask and re-emitted unchanged by ToPaths.
func TestFromPaths_Wildcard_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		paths     []string
		wantPaths []string
	}{
		{
			name:      "leaf wildcard on repeated",
			paths:     []string{"aliases.*"},
			wantPaths: []string{"aliases.*"},
		},
		{
			name:      "nested field after wildcard on repeated",
			paths:     []string{"aliases.*.display_name"},
			wantPaths: []string{"aliases.*.display_name"},
		},
		{
			name:      "wildcard on map plus nested field",
			paths:     []string{"labels.*.display_name"},
			wantPaths: []string{"labels.*.display_name"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mask := fieldmask.FromPaths(tc.paths...)
			got := mask.ToPaths()
			slices.Sort(got)
			require.Equal(t, tc.wantPaths, got)
		})
	}
}

// TestValidate_Wildcard checks validatePath: "*" is accepted after a
// repeated or map field; using it on a singular scalar / message is an
// error (caught by the existing scalar-traversal rule), and putting any
// segment beyond a wildcard on a scalar list is rejected.
func TestValidate_Wildcard(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"leaf wildcard on repeated", "aliases.*", false},
		{"field after wildcard on repeated", "aliases.*.display_name", false},
		{"unknown field after wildcard", "aliases.*.no_such_field", true},
		{"leaf wildcard on map", "labels.*", false},
		{"field after wildcard on map", "labels.*.display_name", false},
		{"wildcard on scalar field is invalid", "name.*", true},
		{"deeper segment after scalar-list wildcard", "tags.*.something", true},
		{"leaf wildcard on scalar list is fine", "tags.*", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := fieldmask.FromPaths(tc.path).Validate(&pb.Resource{})
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

// TestFilter_Wildcard_OnRepeated verifies that "aliases.*.display_name"
// applies the nested mask to every element of the repeated field — the
// canonical AIP-161 form, equivalent to the historical implicit
// "aliases.display_name" semantics this package already supported.
func TestFilter_Wildcard_OnRepeated(t *testing.T) {
	t.Parallel()

	res := &pb.Resource{
		Aliases: []*pb.Profile{
			{Id: "a1", DisplayName: "kept-1"},
			{Id: "a2", DisplayName: "kept-2"},
		},
	}

	fieldmask.FromPaths("aliases.*.display_name").Filter(res)

	require.Len(t, res.GetAliases(), 2)
	for _, a := range res.GetAliases() {
		require.NotEmpty(t, a.GetDisplayName())
		require.Empty(t, a.GetId(), "id was not in mask — must be cleared on every element")
	}
}

// TestFilter_LeafWildcard_OnRepeated covers "aliases.*" as a leaf rule:
// the whole list is kept intact (no per-element filtering).
func TestFilter_LeafWildcard_OnRepeated(t *testing.T) {
	t.Parallel()

	res := &pb.Resource{
		Name: "n",
		Aliases: []*pb.Profile{
			{Id: "a1", DisplayName: "kept"},
		},
	}
	fieldmask.FromPaths("aliases.*").Filter(res)

	require.Equal(t, "", res.GetName(), "fields outside the mask are cleared")
	require.Len(t, res.GetAliases(), 1)
	require.Equal(t, "a1", res.GetAliases()[0].GetId(), "leaf wildcard keeps element fields intact")
}

// TestFilter_Wildcard_OnMap covers the new explicit form: "labels.*"
// applies the wildcard mask to every value in the map. Without the
// wildcard, the existing per-key lookup behavior is preserved.
func TestFilter_Wildcard_OnMap(t *testing.T) {
	t.Parallel()

	res := &pb.Resource{
		Labels: map[string]*pb.Profile{
			"a": {Id: "1", DisplayName: "alpha"},
			"b": {Id: "2", DisplayName: "beta"},
		},
	}

	fieldmask.FromPaths("labels.*.display_name").Filter(res)

	require.Len(t, res.GetLabels(), 2, "wildcard keeps every entry")
	for k, v := range res.GetLabels() {
		require.NotEmpty(t, v.GetDisplayName(), "key %s display_name kept", k)
		require.Empty(t, v.GetId(), "key %s id cleared by wildcard mask", k)
	}
}

// TestFilter_WildcardAndSpecificKey_OnMap: when both "*" and a specific
// key entry are present, the specific entry wins for matched keys and
// the wildcard applies to the rest.
func TestFilter_WildcardAndSpecificKey_OnMap(t *testing.T) {
	t.Parallel()

	res := &pb.Resource{
		Labels: map[string]*pb.Profile{
			"admin": {Id: "i-admin", DisplayName: "Admin"},
			"guest": {Id: "i-guest", DisplayName: "Guest"},
		},
	}

	mask := fieldmask.FromPaths(
		"labels.admin.id",       // specific: keep only id on "admin"
		"labels.*.display_name", // wildcard: keep display_name on others
	)
	mask.Filter(res)

	require.Len(t, res.GetLabels(), 2)
	admin := res.GetLabels()["admin"]
	require.Equal(t, "i-admin", admin.GetId())
	require.Empty(t, admin.GetDisplayName(), "specific rule wins for admin")
	guest := res.GetLabels()["guest"]
	require.Equal(t, "Guest", guest.GetDisplayName())
	require.Empty(t, guest.GetId(), "wildcard governs guest")
}

// TestPrune_Wildcard_OnRepeated mirrors the Filter test: the nested
// mask under "*" is the per-element prune mask.
func TestPrune_Wildcard_OnRepeated(t *testing.T) {
	t.Parallel()

	res := &pb.Resource{
		Aliases: []*pb.Profile{
			{Id: "a1", DisplayName: "to-be-removed-1"},
			{Id: "a2", DisplayName: "to-be-removed-2"},
		},
	}

	fieldmask.FromPaths("aliases.*.display_name").Prune(res)

	require.Len(t, res.GetAliases(), 2, "list itself is kept; only the nested field is pruned per element")
	for _, a := range res.GetAliases() {
		require.NotEmpty(t, a.GetId(), "id was not in prune mask")
		require.Empty(t, a.GetDisplayName(), "display_name pruned by wildcard mask")
	}
}

// TestPrune_LeafWildcard_OnRepeated covers leaf "*" on a repeated field:
// the whole list is cleared (the wildcard says "prune every element").
func TestPrune_LeafWildcard_OnRepeated(t *testing.T) {
	t.Parallel()

	res := &pb.Resource{
		Name: "n",
		Aliases: []*pb.Profile{
			{Id: "a1", DisplayName: "x"},
		},
	}
	fieldmask.FromPaths("aliases.*").Prune(res)

	require.Equal(t, "n", res.GetName(), "fields outside the prune mask are preserved")
	require.Empty(t, res.GetAliases(), "leaf wildcard on repeated clears the list")
}

// TestPrune_LeafWildcard_OnMap: leaf "*" on a map clears every entry.
func TestPrune_LeafWildcard_OnMap(t *testing.T) {
	t.Parallel()

	res := &pb.Resource{
		Labels: map[string]*pb.Profile{
			"a": {Id: "1"},
			"b": {Id: "2"},
		},
	}
	fieldmask.FromPaths("labels.*").Prune(res)

	require.Empty(t, res.GetLabels(), "leaf wildcard prunes every map entry")
}

// TestApplyUpdateMask_Wildcard_StripsOutputOnly is the AIP-161 transitive
// OUTPUT_ONLY strip: when a wildcard sub-mask names an OUTPUT_ONLY field
// on the element type (here Profile.updated_at), ApplyUpdateMask removes
// that path from the writeback mask before returning. If the wildcard
// sub-mask contained only OUTPUT_ONLY entries the parent collection is
// also removed from the mask entirely.
func TestApplyUpdateMask_Wildcard_StripsOutputOnly(t *testing.T) {
	t.Parallel()

	// aliases.*.updated_at — Profile.updated_at is OUTPUT_ONLY. The
	// wildcard sub-mask becomes empty after the strip, so the whole
	// "aliases" entry is dropped. ApplyUpdateMask returns no error.
	t.Run("only OUTPUT_ONLY under wildcard", func(t *testing.T) {
		t.Parallel()

		mask := fieldmask.FromPaths("aliases.*.updated_at")
		err := mask.ApplyUpdateMask(&pb.Resource{Aliases: []*pb.Profile{{Id: "a"}}})
		require.NoError(t, err)
		require.Empty(t, mask.ToPaths(), "wildcard subtree with only OUTPUT_ONLY entries collapses")
	})

	// aliases.*.{updated_at,display_name} — only updated_at is stripped;
	// display_name remains; the parent path stays.
	t.Run("mixed OUTPUT_ONLY and editable under wildcard", func(t *testing.T) {
		t.Parallel()

		mask := fieldmask.FromPaths("aliases.*.updated_at", "aliases.*.display_name")
		err := mask.ApplyUpdateMask(&pb.Resource{Aliases: []*pb.Profile{{DisplayName: "x"}}})
		require.NoError(t, err)
		require.Equal(t, []string{"aliases.*.display_name"}, mask.ToPaths())
	})

	// labels.*.updated_at — same behavior over a map<string, Profile>.
	t.Run("wildcard over map values", func(t *testing.T) {
		t.Parallel()

		mask := fieldmask.FromPaths("labels.*.updated_at", "labels.*.display_name")
		err := mask.ApplyUpdateMask(&pb.Resource{Labels: map[string]*pb.Profile{"k": {DisplayName: "x"}}})
		require.NoError(t, err)
		require.Equal(t, []string{"labels.*.display_name"}, mask.ToPaths())
	})
}

// TestRejectIndexedRepeatedAccess_AllowsWildcard ensures the AIP-161
// write-side guard does not classify "*" as an indexed access — the
// wildcard is the canonical write-mask form for "every element".
func TestRejectIndexedRepeatedAccess_AllowsWildcard(t *testing.T) {
	t.Parallel()

	for _, p := range []string{"aliases.*", "aliases.*.display_name", "labels.*.display_name"} {
		mask := fieldmask.FromPaths(p)
		err := mask.ApplyUpdateMask(&pb.Resource{})
		if err != nil {
			var ve *fieldmask.ValidationError
			if errors.As(err, &ve) {
				require.NotContains(t, ve.Reason, "indexed access",
					"path %q must not be rejected as indexed access", p)
			}
		}
	}
}
