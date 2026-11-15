// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// checkPathName returns [ErrUnaddressableName] for a name that cannot be one
// segment of a document path.
func checkPathName(name string) error {
	if strings.Contains(name, ".") || strings.HasPrefix(name, "$") {
		return fmt.Errorf("%w: %q", ErrUnaddressableName, name)
	}
	return nil
}

// maxValidateDepth bounds validateValue, so a cyclic pointer graph — which
// the codec would also never finish encoding — fails instead of recursing.
const maxValidateDepth = 256

// validateValue walks v along the paths the codec encodes — the fields of
// each struct's layout, non-nil pointer targets, dynamic interface values,
// slice, array and map elements — and runs layoutOf on every struct type it
// meets, so a declaration the codec cannot describe (it recurses forever on a
// struct inlined into itself) fails here with an error. Fields tagged "-",
// nil pointers and types with their own BSON marshaler are not described by
// the codec and are skipped.
func validateValue(v reflect.Value, depth int) error {
	if depth > maxValidateDepth {
		return fmt.Errorf("behavior/mongo: value nests deeper than %d levels", maxValidateDepth)
	}
	if !v.IsValid() || selectsMarshaler(v) {
		return nil
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return validateValue(v.Elem(), depth+1)
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return nil
		}
		for i := range v.Len() {
			if err := validateValue(v.Index(i), depth+1); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		for it := v.MapRange(); it.Next(); {
			if err := validateValue(it.Value(), depth+1); err != nil {
				return err
			}
		}
		return nil
	case reflect.Struct:
		return validateStruct(v, depth)
	default:
		return nil
	}
}

// validateStruct is validateValue for a struct: it checks the struct's
// layout and walks the fields the codec encodes and its inline map.
func validateStruct(v reflect.Value, depth int) error {
	layout, err := layoutOf(v.Type(), DefaultBSONTagName)
	if err != nil {
		return err
	}
	for _, df := range layout.fields {
		fv, err := v.FieldByIndexErr(df.index)
		if err != nil {
			continue // behind a nil inline pointer: not encoded
		}
		if df.omitEmpty && isCodecEmpty(fv) {
			continue // omitted: not encoded
		}
		if err := validateValue(fv, depth+1); err != nil {
			return err
		}
	}
	if layout.inlineMap >= 0 {
		return validateValue(v.Field(layout.inlineMap), depth+1)
	}
	return nil
}

// staticSafeCache memoizes staticallySafe per type.
var staticSafeCache sync.Map // map[reflect.Type]bool

// staticallySafe reports whether every struct type a value of type t can
// reach — through layout fields, pointers, slices, arrays and maps — has a
// valid layout and no interface sits on the way, so no value of t can lead
// the codec into a declaration it cannot describe. Such types skip the
// per-value [validateValue] walk; any other type gets it, which also accepts
// the invalid types a given value never encodes (nil, omitted or ignored).
func staticallySafe(t reflect.Type) bool {
	if v, ok := staticSafeCache.Load(t); ok {
		b, _ := v.(bool)
		return b
	}
	b := staticWalk(t, map[reflect.Type]walkState{})
	staticSafeCache.Store(t, b)
	return b
}

// walkState tracks a type during staticWalk.
type walkState uint8

const (
	walkActive walkState = iota + 1 // on the current path
	walkDone                        // finished and safe
)

// staticWalk is staticallySafe with cycle detection. A type met again while
// still on the current path makes the graph recursive: its values can nest
// without bound or form a cycle, so they keep the per-value walk, which
// bounds the depth. A type already finished is simply safe.
func staticWalk(t reflect.Type, seen map[reflect.Type]walkState) bool {
	switch seen[t] {
	case walkActive:
		return false
	case walkDone:
		return true
	}
	seen[t] = walkActive
	if !staticKind(t, seen) {
		return false
	}
	seen[t] = walkDone
	return true
}

// staticKind checks one type's layers for staticWalk.
func staticKind(t reflect.Type, seen map[reflect.Type]walkState) bool {
	switch t.Kind() {
	case reflect.Interface:
		return false
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return staticWalk(t.Elem(), seen)
	case reflect.Struct:
		if implementsBSONMarshaler(t) {
			// Encoded by its marshaler only when addressable; stay
			// conservative and let the value walk decide.
			return false
		}
		layout, err := layoutOf(t, DefaultBSONTagName)
		if err != nil {
			return false
		}
		for _, df := range layout.fields {
			if !staticWalk(t.FieldByIndex(df.index).Type, seen) {
				return false
			}
		}
		if layout.inlineMap >= 0 {
			return staticWalk(t.Field(layout.inlineMap).Type, seen)
		}
		return true
	default:
		return true
	}
}

// selectsMarshaler reports whether the codec encodes v with a BSON marshaler
// rather than describing its type: the type implements one itself, or v is
// addressable and its pointer does.
func selectsMarshaler(v reflect.Value) bool {
	t := v.Type()
	if t.Implements(bsonMarshalerType) || t.Implements(bsonValueMarshalerType) {
		return true
	}
	return v.CanAddr() && implementsBSONMarshaler(t)
}

// rejectOperatorKeys fails on a key starting with "$" in doc or any document
// nested in it, including documents inside arrays and the scope of JavaScript
// code with scope.
func rejectOperatorKeys(doc bson.Raw) error {
	elems, err := doc.Elements()
	if err != nil {
		return fmt.Errorf("behavior/mongo: insert document: %w", err)
	}
	for _, e := range elems {
		if key := e.Key(); strings.HasPrefix(key, "$") {
			return fmt.Errorf("behavior/mongo: key %q must not start with %q (reserved for MongoDB operators)", key, "$")
		}
		val := e.Value()
		switch val.Type {
		case bson.TypeEmbeddedDocument:
			if err := rejectOperatorKeys(val.Document()); err != nil {
				return err
			}
		case bson.TypeArray:
			if err := rejectOperatorKeys(bson.Raw(val.Array())); err != nil {
				return err
			}
		case bson.TypeCodeWithScope:
			// The scope of JavaScript code with scope is a document too.
			if _, scope, ok := val.CodeWithScopeOK(); ok {
				if err := rejectOperatorKeys(scope); err != nil {
					return err
				}
			}
		default:
		}
	}
	return nil
}
