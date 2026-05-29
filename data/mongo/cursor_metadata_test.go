// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestComputeFilterHash_StableAcrossMapIteration confirms the filter hash is
// deterministic: Go map iteration is randomized, so a filter with a nested
// bson.M of >=2 keys must hash the same across calls — otherwise cursor
// pagination intermittently fails with INVALID_CURSOR.
func TestComputeFilterHash_StableAcrossMapIteration(t *testing.T) {
	filter := bson.M{"$and": bson.A{
		bson.M{"deleted_at": nil, "organization_id": "f3360599-53c9-4833-9bb6-3c75e043caea"},
		bson.M{"cooperation_format": int64(3)},
	}}

	first := computeFilterHash(filter)
	for range 1000 {
		require.Equal(t, first, computeFilterHash(filter),
			"same filter must produce the same hash regardless of map iteration order")
	}
}
