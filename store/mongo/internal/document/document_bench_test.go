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

func BenchmarkMatches(b *testing.B) {
	type entity struct {
		Email string `encryption:"deterministic,email"`
	}
	typ := reflect.TypeFor[entity]()
	sf := typ.Field(0)
	fields, err := document.New("encryption", nil, nil)
	require.NoError(b, err)
	for b.Loop() {
		fields.Matches(typ, sf)
	}
}

func BenchmarkTransformedInsert(b *testing.B) {
	type child struct {
		Token string `encryption:"random,key"`
	}
	entity := struct {
		Name     string
		Children []child
	}{"example", []child{{"one"}, {"two"}}}
	ciphertext := bson.Binary{Subtype: bson.TypeBinaryEncrypted, Data: []byte{1}}
	fields, err := document.New("encryption", nil, func(context.Context, bson.RawValue, string, string) (bson.Binary, error) {
		return ciphertext, nil
	})
	require.NoError(b, err)
	ctx := bsonfield.WithTransformer(b.Context(), fields)
	engine := behavior.New(mongo.NewInsertTranslator())
	b.ReportAllocs()
	for b.Loop() {
		if _, err := engine.Translate(ctx, entity); err != nil {
			b.Fatal(err)
		}
	}
}
