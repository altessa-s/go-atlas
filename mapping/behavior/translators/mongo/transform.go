// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"fmt"
	"reflect"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas-core/types/optional"
	"github.com/altessa-s/go-atlas/internal/bsonfield"
	"github.com/altessa-s/go-atlas/mapping/behavior"
)

// fieldTransforms belongs to one translation. A whole-value codec may run
// only after proving it cannot bypass a selected field, even in a dynamic
// interface or in private storage exposed by a custom encoder (e.g. Optional).
type fieldTransforms struct {
	ctx context.Context
	bsonfield.Transformer
	types map[reflect.Type]bsonfield.Selection
}

func (ft *fieldTransforms) selection(t reflect.Type) bsonfield.Selection {
	s, ok := ft.types[t]
	if !ok {
		s = bsonfield.Inspect(t, ft.Matches)
		ft.types[t] = s
	}
	return s
}

// required checks concrete values only where interfaces leave the static type
// inconclusive. Only reflection on structure and types is needed, including
// for private values. Never invoke a user marshaler to discover its shape.
func (ft *fieldTransforms) required(v reflect.Value, depth int, insideSelected bool) bool {
	if !v.IsValid() {
		return false
	}
	t := v.Type()
	s := ft.selection(t)
	if insideSelected && s >= bsonfield.SelectedFields {
		return true // A nil or absent value still has its declared rules.
	}
	if s == bsonfield.NoFields || (v.Kind() == reflect.Pointer && v.IsNil()) {
		return false
	}
	if depth >= maxValidateDepth {
		return true
	}
	if v.CanInterface() && optional.IsOptionalType(t) {
		inner, present := optional.GetReflect(v)
		return present && ft.required(inner, depth+1, insideSelected)
	}
	if s >= bsonfield.SelectedFields {
		return true
	}
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		return ft.required(v.Elem(), depth+1, insideSelected)
	case reflect.Map:
		for iter := v.MapRange(); iter.Next(); {
			if ft.required(iter.Key(), depth+1, insideSelected) || ft.required(iter.Value(), depth+1, insideSelected) {
				return true
			}
		}
	case reflect.Array, reflect.Slice:
		for i := range v.Len() {
			if ft.required(v.Index(i), depth+1, insideSelected) {
				return true
			}
		}
	case reflect.Struct:
		for i := range t.NumField() {
			if ft.required(v.Field(i), depth+1, insideSelected) {
				return true
			}
		}
	default:
	}
	return false
}

func (ft *fieldTransforms) opaque(v reflect.Value) error {
	if ft != nil && ft.required(v, 0, false) {
		return fmt.Errorf("%w: %s is encoded as a unit", bsonfield.ErrFieldTransform, v.Type())
	}
	return nil
}

// selected checks every ancestor imported by inline. Such a container has no
// stored key to receive a replacement, so flattening it would lose the transform.
func (ft *fieldTransforms) selected(owner reflect.Type, df docField) (bool, error) {
	t := owner
	for j, index := range df.index {
		f := t.Field(index)
		if ft.Matches(t, f) {
			if j != len(df.index)-1 {
				return false, fmt.Errorf("%w: inline field %s.%s has no BSON key", bsonfield.ErrFieldTransform, t, f.Name)
			}
			return true, nil
		}
		t = f.Type
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
	}
	return false, nil
}

func (ft *fieldTransforms) replace(df docField, v reflect.Value, ec encCtx) (any, error) {
	if (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) && v.IsNil() {
		return nil, nil //nolint:nilnil // BSON null has no value to transform.
	}
	if ft.required(v, 0, true) {
		return nil, fmt.Errorf("%w: selected field %s.%s contains another selected field", bsonfield.ErrFieldTransform, ec.owner, df.name)
	}
	return ft.Transform(ft.ctx, ec.owner, df.index, df.name, v, ec.reg, func() (bson.RawValue, error) {
		return encodeTransformValue(v, ec)
	})
}

// encodeTransformValue checks the layout and depth before handing an entire
// field to the codec, just as encodeInsert does for an entire document.
func encodeTransformValue(v reflect.Value, ec encCtx) (bson.RawValue, error) {
	if err := validateCodecInput(v); err != nil {
		return bson.RawValue{}, err
	}
	return encodeWrapped(v, ec.minSize, v.CanAddr() && !ec.noAddr, ec.reg)
}

// orderedDocument restores the struct codec's key order for an insert
// subdocument after folding selected fields. The driver appends inline map
// entries after the declared fields, in unspecified order.
func (o *options) orderedDocument(obj behavior.Object, sub bson.M) (bson.D, error) {
	layout, err := o.objectLayout(obj)
	if err != nil {
		return nil, err
	}
	ordered := make(bson.D, 0, len(sub))
	for _, field := range layout.fields {
		if value, ok := sub[field.name]; ok {
			ordered = append(ordered, bson.E{Key: field.name, Value: value})
		}
	}
	if layout.inlineMap >= 0 {
		for name, value := range sub {
			if _, declared := layout.names[name]; !declared {
				ordered = append(ordered, bson.E{Key: name, Value: value})
			}
		}
	}
	return ordered, nil
}
