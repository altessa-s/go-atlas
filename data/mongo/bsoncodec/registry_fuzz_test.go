// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsoncodec_test

import (
	"github.com/altessa-s/go-atlas/core/types/optional"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"testing"
)

// FuzzBSONRoundTrip is the same property for the encoding MongoDB documents use.
func FuzzBSONRoundTrip(f *testing.F) {
	f.Add(0, true)
	f.Add(42, true)
	f.Add(-7, false)

	f.Fuzz(func(t *testing.T, v int, present bool) {
		original := optional.Of(v, present)

		raw, err := marshal(bson.M{"value": original})
		require.NoError(t, err)

		var out struct {
			Value optional.Optional[int] `bson:"value"`
		}
		require.NoError(t, unmarshal(raw, &out))

		require.Equal(t, original, out.Value, "the value changed on its way through BSON")
		require.Equal(t, original.IsNone(), out.Value.IsZero(),
			"IsZero must agree with IsNone, or omitempty drops a present zero value")
	})
}
