// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package optional_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/types/optional"
)

func TestIsZero(t *testing.T) {
	t.Parallel()

	require.True(t, optional.None[int]().IsZero())
	require.True(t, optional.None[string]().IsZero())
	require.True(t, optional.Optional[time.Time]{}.IsZero())

	require.False(t, optional.Some(0).IsZero())
	require.False(t, optional.Some("").IsZero())
	require.False(t, optional.Some(time.Time{}).IsZero())
}

func TestMarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("none_emits_null", func(t *testing.T) {
		t.Parallel()

		raw, err := optional.None[int]().MarshalJSON()
		require.NoError(t, err)
		require.Equal(t, []byte("null"), raw)
	})

	t.Run("some_emits_underlying", func(t *testing.T) {
		t.Parallel()

		raw, err := optional.Some(42).MarshalJSON()
		require.NoError(t, err)
		require.Equal(t, []byte("42"), raw)
	})

	t.Run("some_empty_string", func(t *testing.T) {
		t.Parallel()

		raw, err := optional.Some("").MarshalJSON()
		require.NoError(t, err)
		require.Equal(t, []byte(`""`), raw)
	})
}

func TestUnmarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("null_becomes_none", func(t *testing.T) {
		t.Parallel()

		o := optional.Some("seed")
		require.NoError(t, o.UnmarshalJSON([]byte("null")))
		require.True(t, o.IsNone())
		require.Equal(t, "", o.Value())
	})

	t.Run("value_becomes_some", func(t *testing.T) {
		t.Parallel()

		var o optional.Optional[int]
		require.NoError(t, o.UnmarshalJSON([]byte("42")))
		require.True(t, o.IsSome())
		require.Equal(t, 42, o.Value())
	})

	t.Run("decode_error_preserves_state", func(t *testing.T) {
		t.Parallel()

		o := optional.Some(7)
		require.Error(t, o.UnmarshalJSON([]byte("not-json")))
		v, ok := o.Get()
		require.True(t, ok)
		require.Equal(t, 7, v)
	})
}

func TestJSONRoundTrip_StructField(t *testing.T) {
	t.Parallel()

	type doc struct {
		Note optional.Optional[string] `json:"note"`
	}

	t.Run("some", func(t *testing.T) {
		t.Parallel()

		raw, err := json.Marshal(doc{Note: optional.Some("hi")})
		require.NoError(t, err)
		require.JSONEq(t, `{"note":"hi"}`, string(raw))

		var out doc
		require.NoError(t, json.Unmarshal(raw, &out))
		v, ok := out.Note.Get()
		require.True(t, ok)
		require.Equal(t, "hi", v)
	})

	t.Run("none_emits_null", func(t *testing.T) {
		t.Parallel()

		raw, err := json.Marshal(doc{Note: optional.None[string]()})
		require.NoError(t, err)
		require.JSONEq(t, `{"note":null}`, string(raw))
	})

	t.Run("missing_field_decodes_as_none", func(t *testing.T) {
		t.Parallel()

		var out doc
		require.NoError(t, json.Unmarshal([]byte(`{}`), &out))
		require.True(t, out.Note.IsNone())
	})

	t.Run("explicit_null_decodes_as_none", func(t *testing.T) {
		t.Parallel()

		var out doc
		require.NoError(t, json.Unmarshal([]byte(`{"note":null}`), &out))
		require.True(t, out.Note.IsNone())
	})
}
