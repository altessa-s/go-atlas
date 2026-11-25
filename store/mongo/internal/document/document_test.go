// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package document_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/internal/bsonfield"
	"github.com/altessa-s/go-atlas/mapping/behavior"
	"github.com/altessa-s/go-atlas/mapping/behavior/translators/mongo"
	"github.com/altessa-s/go-atlas/store/mongo/internal/document"
)

func TestModelRuleThroughInlinePointer(t *testing.T) {
	t.Parallel()
	type embedded struct {
		Secret string `encryption:"random,tag"`
	}
	type root struct {
		P *embedded `bson:",inline"`
	}
	typ := reflect.TypeFor[embedded]()
	model := &document.EncryptionModel{Fields: []document.EncryptionField{{FieldName: "Secret", Algorithm: "deterministic", KeyAltName: "model"}}}
	raw := bson.RawValue{Type: bson.TypeBoolean, Value: []byte{0}}
	want := bson.Binary{Subtype: bson.TypeBinaryEncrypted, Data: []byte("ciphertext")}
	fields, err := document.New("encryption", map[reflect.Type]*document.EncryptionModel{typ: model},
		func(_ context.Context, v bson.RawValue, alg, key string) (bson.Binary, error) {
			require.Equal(t, raw, v)
			require.Equal(t, "deterministic", alg)
			require.Equal(t, "model", key)
			return want, nil
		})
	require.NoError(t, err)
	model.Fields[0].KeyAltName = "mutated"
	require.True(t, fields.Matches(typ, typ.Field(0)))
	got, err := fields.Transform(t.Context(), reflect.TypeFor[root](), []int{0, 0}, "stored",
		func() (bson.RawValue, error) { return raw, nil })
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestRejectInvalidModel(t *testing.T) {
	t.Parallel()
	type Embedded struct{ Secret string }
	type root struct{ Embedded }
	for _, tc := range []struct {
		name   string
		typ    reflect.Type
		fields []document.EncryptionField
	}{
		{"missing type", nil, nil},
		{"non-struct", reflect.TypeFor[int](), nil},
		{"promoted field", reflect.TypeFor[root](), []document.EncryptionField{{FieldName: "Secret", Algorithm: "random", KeyAltName: "k"}}},
		{"unknown field", reflect.TypeFor[root](), []document.EncryptionField{{FieldName: "Missing", Algorithm: "random", KeyAltName: "k"}}},
		{"unknown algorithm", reflect.TypeFor[Embedded](), []document.EncryptionField{{FieldName: "Secret", Algorithm: "invalid", KeyAltName: "k"}}},
		{"no key", reflect.TypeFor[Embedded](), []document.EncryptionField{{FieldName: "Secret", Algorithm: "random"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := document.New("encryption", map[reflect.Type]*document.EncryptionModel{tc.typ: {Fields: tc.fields}}, nil)
			require.ErrorIs(t, err, bsonfield.ErrFieldTransform)
		})
	}
}

func TestInvalidCiphertextReturnsNoDocument(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		value bson.Binary
	}{
		{"wrong subtype", bson.Binary{Data: []byte{1}}},
		{"empty ciphertext", bson.Binary{Subtype: bson.TypeBinaryEncrypted}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fields, err := document.New("encryption", nil, func(context.Context, bson.RawValue, string, string) (bson.Binary, error) {
				return tc.value, nil
			})
			require.NoError(t, err)
			opt := mongo.WithFieldTransformer(fields)
			for _, translator := range []behavior.Translator[bson.M]{mongo.NewInsertTranslator(opt), mongo.NewUpdateTranslator(opt)} {
				doc, err := behavior.New(translator).Translate(t.Context(), struct {
					Secret string `encryption:"random,key"`
				}{"value"})
				require.ErrorIs(t, err, bsonfield.ErrFieldTransform)
				require.Nil(t, doc)
			}
		})
	}
}
