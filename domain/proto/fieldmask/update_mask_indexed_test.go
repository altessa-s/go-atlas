// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

// TestApplyUpdateMask_RejectsIndexedRepeatedAccess locks the AIP-161 rule
// that update masks cannot address an individual element of a repeated
// field. The downstream FieldBehavior validator does not see these paths;
// the new rejectIndexedRepeatedAccess pass returns ValidationError before
// FieldBehavior runs.
func TestApplyUpdateMask_RejectsIndexedRepeatedAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		path      string
		wantError bool
	}{
		{"index on repeated message", "aliases.0", true},
		{"index on repeated then field", "aliases.0.display_name", true},
		{"index at deep position", "aliases.10.id", true},
		{"index on scalar-repeated", "tags.0", true},
		{"whole repeated field is fine", "aliases", false},
		{"field on element is fine", "aliases.display_name", false},
		{"map string key is fine", "labels.alice", false},
		{"map string key with field is fine", "labels.alice.display_name", false},
		{"map numeric-looking key is fine", "labels.42", false},
		{"map numeric-looking key with field is fine", "labels.42.display_name", false},
		{"plain field path is fine", "name", false},
		{"unknown field is silently allowed", "definitely_not_a_field", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mask := fieldmask.FromPaths(tc.path)
			err := mask.ApplyUpdateMask(&pb.Resource{})

			if tc.wantError {
				require.Error(t, err)
				var ve *fieldmask.ValidationError
				require.True(t, errors.As(err, &ve),
					"want *fieldmask.ValidationError, got %T (%v)", err, err)
				require.Equal(t, tc.path, ve.Path)
				require.Contains(t, ve.Reason, "indexed access")
				return
			}

			// Non-rejected paths may still fail downstream for other reasons
			// (unknown field, behavior violation). The contract here is only
			// that they must NOT fail with the indexed-access ValidationError.
			if err != nil {
				var ve *fieldmask.ValidationError
				if errors.As(err, &ve) {
					require.NotContains(t, ve.Reason, "indexed access",
						"path %q should not be rejected for indexed access", tc.path)
				}
			}
		})
	}
}

// TestApplyUpdateMask_IndexInsideMapValue covers the deep case where the
// indexed segment sits beneath a map value — the walker must still descend
// into the map-value descriptor to reach the nested repeated field.
func TestApplyUpdateMask_IndexInsideMapValue(t *testing.T) {
	t.Parallel()

	// `labels` is map<string, Profile>; Profile has no repeated of its own,
	// so a synthetic `labels.alice.0` lands on a non-repeated segment and
	// must NOT be rejected as indexed access. It will still be a no-op
	// downstream (unknown field on Profile), but that is not our concern.
	mask := fieldmask.FromPaths("labels.alice.0")
	err := mask.ApplyUpdateMask(&pb.Resource{})

	if err != nil {
		var ve *fieldmask.ValidationError
		if errors.As(err, &ve) {
			require.NotContains(t, ve.Reason, "indexed access")
		}
	}
}
