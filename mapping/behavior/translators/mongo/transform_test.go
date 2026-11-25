// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"
	"reflect"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/internal/bsonfield"
	"github.com/altessa-s/go-atlas/mapping/behavior"
	"github.com/altessa-s/go-atlas/mapping/behavior/translators/mongo"
)

type prefixKey struct{}

type prefixTransformer struct{}

func (*prefixTransformer) Matches(_ reflect.Type, field reflect.StructField) bool {
	return field.Name == "Message"
}

func (*prefixTransformer) Transform(ctx context.Context, _ reflect.Type, _ []int, _ string, encode func() (bson.RawValue, error)) (bson.Binary, error) {
	raw, err := encode()
	if err != nil {
		return bson.Binary{}, err
	}
	prefix, _ := ctx.Value(prefixKey{}).(string)
	return bson.Binary{Data: []byte(prefix + raw.StringValue())}, nil
}

func TestTranslate_FieldTransformerParallel(t *testing.T) {
	t.Parallel()
	engine := behavior.New(mongo.NewInsertTranslator(mongo.WithFieldTransformer(&prefixTransformer{})))
	for i := range 16 {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			ctx := context.WithValue(t.Context(), prefixKey{}, t.Name())
			doc, err := engine.Translate(ctx, struct{ Message string }{"message"})
			require.NoError(t, err)
			require.Equal(t, bson.Binary{Data: []byte(t.Name() + "message")}, doc["message"])
		})
	}
}

func TestTranslate_RejectsNilTransformer(t *testing.T) {
	t.Parallel()
	var transformer *prefixTransformer
	opt := mongo.WithFieldTransformer(transformer)
	for _, translator := range []behavior.Translator[bson.M]{mongo.NewInsertTranslator(opt), mongo.NewUpdateTranslator(opt)} {
		doc, err := behavior.New(translator).Translate(t.Context(), struct{ Message string }{"message"})
		require.ErrorIs(t, err, bsonfield.ErrFieldTransform)
		require.Nil(t, doc)
	}
}
