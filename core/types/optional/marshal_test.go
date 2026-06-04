// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package optional_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

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

func TestMarshalBSONValue_NoneEmitsNull(t *testing.T) {
	t.Parallel()

	typ, data, err := optional.None[int]().MarshalBSONValue()
	require.NoError(t, err)
	require.Equal(t, byte(bson.TypeNull), typ)
	require.Nil(t, data)
}

func TestMarshalBSONValue_SomePassesThrough(t *testing.T) {
	t.Parallel()

	wantType, wantData, err := bson.MarshalValue(int64(42))
	require.NoError(t, err)

	gotType, gotData, err := optional.Some[int64](42).MarshalBSONValue()
	require.NoError(t, err)
	require.Equal(t, byte(wantType), gotType)
	require.Equal(t, wantData, gotData)
}

func TestUnmarshalBSONValue_NullBecomesNone(t *testing.T) {
	t.Parallel()

	o := optional.Some("seed")
	require.NoError(t, o.UnmarshalBSONValue(byte(bson.TypeNull), nil))
	require.True(t, o.IsNone())
	require.Equal(t, "", o.Value())
}

func TestUnmarshalBSONValue_SomeRoundTrip(t *testing.T) {
	t.Parallel()

	typ, data, err := bson.MarshalValue("hello")
	require.NoError(t, err)

	var o optional.Optional[string]
	require.NoError(t, o.UnmarshalBSONValue(byte(typ), data))
	require.True(t, o.IsSome())
	require.Equal(t, "hello", o.Value())
}

func TestBSONRoundTrip_StructField(t *testing.T) {
	t.Parallel()

	type doc struct {
		DeletedAt optional.Optional[time.Time] `bson:"deleted_at,omitempty"`
		Note      optional.Optional[string]    `bson:"note,omitempty"`
	}

	t.Run("some_round_trips", func(t *testing.T) {
		t.Parallel()

		// Truncate to BSON's millisecond datetime precision.
		when := time.Now().UTC().Truncate(time.Millisecond)
		in := doc{
			DeletedAt: optional.Some(when),
			Note:      optional.Some(""),
		}

		raw, err := bson.Marshal(in)
		require.NoError(t, err)

		var out doc
		require.NoError(t, bson.Unmarshal(raw, &out))

		gotTime, ok := out.DeletedAt.Get()
		require.True(t, ok)
		require.True(t, when.Equal(gotTime), "want %v got %v", when, gotTime)

		gotNote, ok := out.Note.Get()
		require.True(t, ok, "Some(\"\") must round-trip as Some, not None")
		require.Equal(t, "", gotNote)
	})

	t.Run("none_is_omitted_with_omitempty", func(t *testing.T) {
		t.Parallel()

		raw, err := bson.Marshal(doc{})
		require.NoError(t, err)

		var m bson.M
		require.NoError(t, bson.Unmarshal(raw, &m))
		require.NotContains(t, m, "deleted_at")
		require.NotContains(t, m, "note")
	})

	t.Run("missing_field_decodes_as_none", func(t *testing.T) {
		t.Parallel()

		raw, err := bson.Marshal(bson.M{})
		require.NoError(t, err)

		var out doc
		require.NoError(t, bson.Unmarshal(raw, &out))
		require.True(t, out.DeletedAt.IsNone())
		require.True(t, out.Note.IsNone())
	})

	t.Run("explicit_null_decodes_as_none", func(t *testing.T) {
		t.Parallel()

		raw, err := bson.Marshal(bson.M{"deleted_at": nil, "note": nil})
		require.NoError(t, err)

		var out doc
		require.NoError(t, bson.Unmarshal(raw, &out))
		require.True(t, out.DeletedAt.IsNone())
		require.True(t, out.Note.IsNone())
	})
}

func TestBSONRoundTrip_PointerInsideOptional(t *testing.T) {
	t.Parallel()

	type inner struct {
		Label string `bson:"label"`
	}
	type doc struct {
		Nested optional.Optional[*inner] `bson:"nested"`
	}

	in := doc{Nested: optional.Some(&inner{Label: "hi"})}
	raw, err := bson.Marshal(in)
	require.NoError(t, err)

	var out doc
	require.NoError(t, bson.Unmarshal(raw, &out))
	got, ok := out.Nested.Get()
	require.True(t, ok)
	require.NotNil(t, got)
	require.Equal(t, "hi", got.Label)
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
