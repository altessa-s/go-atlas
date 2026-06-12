// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/behavior"
)

// TestStripPointerValuedMap covers the byPointer branch of walkMap: struct
// values reached through a map[K]*T pointer are mutated in place.
func TestStripPointerValuedMap(t *testing.T) {
	t.Parallel()

	type inner struct {
		Secret string `behavior:"input_only"`
		Keep   string
	}
	type holder struct {
		ByPtr map[string]*inner
	}

	h := &holder{ByPtr: map[string]*inner{"a": {Secret: "s", Keep: "k"}}}
	require.NoError(t, behavior.StripResponse(h))

	require.Empty(t, h.ByPtr["a"].Secret, "tagged field reached via map pointer cleared in place")
	require.Equal(t, "k", h.ByPtr["a"].Keep, "untagged field preserved")
}
