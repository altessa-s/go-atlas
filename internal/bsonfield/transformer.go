// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsonfield

import (
	"context"
	"errors"
	"reflect"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ErrFieldTransform identifies a field that the builder could not transform.
var ErrFieldTransform = errors.New("cannot transform BSON field")

// Transformer selects fields by declaring type. Transform receives the enclosing
// document type and Go field index path (including inline ancestors), the stored
// name, and a lazy encoder bound to the field's registry, minsize and addressability.
type Transformer interface {
	Matches(reflect.Type, reflect.StructField) bool
	Transform(ctx context.Context, owner reflect.Type, path []int, name string, value reflect.Value,
		registry *bson.Registry, encode func() (bson.RawValue, error)) (bson.Binary, error)
}

type transformerKey struct{}

// WithTransformer binds t to one conversion.
func WithTransformer(ctx context.Context, t Transformer) context.Context {
	return context.WithValue(ctx, transformerKey{}, t)
}

// FromContext returns the writer's transformer, when configured.
func FromContext(ctx context.Context) Transformer {
	t, _ := ctx.Value(transformerKey{}).(Transformer)
	return t
}
