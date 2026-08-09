// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package optional_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/types/optional"
)

// FuzzSomeZeroIsNotNone is the distinction the type exists for: a value that
// happens to be the zero value is still a value.
//
// A *T or a (T, bool) collapses "absent" and "zero" the moment someone writes
// the wrong check, and the resulting bug — a config field explicitly set to 0
// silently taking the default — is invisible at the call site. The property has
// to hold across the encodings too, since that is where the two states are most
// easily flattened into the same bytes.
func FuzzSomeZeroIsNotNone(f *testing.F) {
	f.Add(0)
	f.Add(1)
	f.Add(-1)

	f.Fuzz(func(t *testing.T, v int) {
		some := optional.Some(v)
		none := optional.None[int]()

		require.True(t, some.IsSome())
		require.True(t, none.IsNone())
		require.NotEqual(t, some, none, "Some(%v) must not equal None", v)

		// OrDefault is where the two states become observable to a caller.
		require.Equal(t, v, some.OrDefault(v+1))
		require.Equal(t, v+1, none.OrDefault(v+1))
	})
}

// FuzzJSONRoundTrip pins that both states survive the wire.
//
// Some(zero) encoding to the same bytes as None is the failure that turns this
// type back into the *T it replaced — and it would only show up for the zero
// value, which is exactly the case a hand-written table forgets.
func FuzzJSONRoundTrip(f *testing.F) {
	f.Add(0, true)
	f.Add(42, true)
	f.Add(-7, false)

	f.Fuzz(func(t *testing.T, v int, present bool) {
		original := optional.Of(v, present)

		encoded, err := json.Marshal(original)
		require.NoError(t, err)

		var decoded optional.Optional[int]
		require.NoError(t, json.Unmarshal(encoded, &decoded))

		require.Equal(t, original, decoded, "the value changed on its way through JSON: %s", encoded)
	})
}

// FuzzBSONRoundTrip is the same property for the encoding MongoDB documents use.
func FuzzBSONRoundTrip(f *testing.F) {
	f.Add(0, true)
	f.Add(42, true)
	f.Add(-7, false)

	f.Fuzz(func(t *testing.T, v int, present bool) {
		original := optional.Of(v, present)

		typ, raw, err := original.MarshalBSONValue()
		require.NoError(t, err)

		var decoded optional.Optional[int]
		require.NoError(t, decoded.UnmarshalBSONValue(typ, raw))

		require.Equal(t, original, decoded, "the value changed on its way through BSON")
		require.Equal(t, original.IsNone(), decoded.IsZero(),
			"IsZero must agree with IsNone, or omitempty drops a present zero value")
	})
}
