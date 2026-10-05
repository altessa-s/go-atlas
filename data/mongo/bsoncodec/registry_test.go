// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsoncodec_test

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/core/types/optional"
	"github.com/altessa-s/go-atlas/core/types/redacted"
	"github.com/altessa-s/go-atlas/data/mongo/bsoncodec"
)

func marshal(v any) ([]byte, error) {
	var out bytes.Buffer
	enc := bson.NewEncoder(bson.NewDocumentWriter(&out))
	enc.SetRegistry(bsoncodec.NewRegistry())
	err := enc.Encode(v)
	return out.Bytes(), err
}

func unmarshal(data []byte, v any) error {
	dec := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(data)))
	dec.SetRegistry(bsoncodec.NewRegistry())
	return dec.Decode(v)
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

		raw, err := marshal(in)
		require.NoError(t, err)

		var out doc
		require.NoError(t, unmarshal(raw, &out))

		gotTime, ok := out.DeletedAt.Get()
		require.True(t, ok)
		require.True(t, when.Equal(gotTime), "want %v got %v", when, gotTime)

		gotNote, ok := out.Note.Get()
		require.True(t, ok, "Some(\"\") must round-trip as Some, not None")
		require.Equal(t, "", gotNote)
	})

	t.Run("none_is_omitted_with_omitempty", func(t *testing.T) {
		t.Parallel()

		raw, err := marshal(doc{})
		require.NoError(t, err)

		var m bson.M
		require.NoError(t, unmarshal(raw, &m))
		require.NotContains(t, m, "deleted_at")
		require.NotContains(t, m, "note")
	})

	t.Run("missing_field_decodes_as_none", func(t *testing.T) {
		t.Parallel()

		raw, err := marshal(bson.M{})
		require.NoError(t, err)

		var out doc
		require.NoError(t, unmarshal(raw, &out))
		require.True(t, out.DeletedAt.IsNone())
		require.True(t, out.Note.IsNone())
	})

	t.Run("explicit_null_decodes_as_none", func(t *testing.T) {
		t.Parallel()

		raw, err := marshal(bson.M{"deleted_at": nil, "note": nil})
		require.NoError(t, err)

		var out doc
		require.NoError(t, unmarshal(raw, &out))
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
	raw, err := marshal(in)
	require.NoError(t, err)

	var out doc
	require.NoError(t, unmarshal(raw, &out))
	got, ok := out.Nested.Get()
	require.True(t, ok)
	require.NotNil(t, got)
	require.Equal(t, "hi", got.Label)
}

func TestUnconfiguredRegistryRejectsOptional(t *testing.T) {
	t.Parallel()
	_, err := bson.Marshal(bson.M{"value": optional.Some(42)})
	require.ErrorIs(t, err, optional.ErrBSONCodecRequired)
	raw, err := bson.Marshal(bson.M{"value": 42})
	require.NoError(t, err)
	var out struct {
		Value optional.Optional[int] `bson:"value"`
	}
	require.ErrorIs(t, bson.Unmarshal(raw, &out), optional.ErrBSONCodecRequired)
}

func TestCustomInnerCodecAndOtherMarshalers(t *testing.T) {
	t.Parallel()
	reg := bsoncodec.NewRegistry()
	reg.RegisterTypeEncoder(reflect.TypeFor[int](), bson.ValueEncoderFunc(func(_ bson.EncodeContext, w bson.ValueWriter, _ reflect.Value) error { return w.WriteString("custom") }))
	var out bytes.Buffer
	enc := bson.NewEncoder(bson.NewDocumentWriter(&out))
	enc.SetRegistry(reg)
	require.NoError(t, enc.Encode(bson.M{"value": optional.Some(42), "secret": redacted.RedactedString("secret")}))
	raw := bson.Raw(out.Bytes())
	require.Equal(t, "custom", raw.Lookup("value").StringValue())
	require.Equal(t, "<redacted>", raw.Lookup("secret").StringValue())
	var decoded struct {
		Secret redacted.RedactedString `bson:"secret"`
	}
	require.NoError(t, unmarshal(out.Bytes(), &decoded))
	require.Equal(t, "<redacted>", decoded.Secret.Expose())
}

func TestDecodeErrorPreservesValue(t *testing.T) {
	t.Parallel()
	in := struct {
		Value optional.Optional[int] `bson:"value"`
	}{optional.Some(7)}
	raw, err := bson.Marshal(bson.M{"value": "invalid"})
	require.NoError(t, err)
	require.Error(t, unmarshal(raw, &in))
	require.Equal(t, optional.Some(7), in.Value)
}
