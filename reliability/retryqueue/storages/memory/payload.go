// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"maps"
	"reflect"
)

// payloadReference identifies an allocation when cloning a cyclic value.
// Slices starting at the same element can expose different lengths.
type payloadReference struct {
	typ reflect.Type
	ptr uintptr
	len int
}

func referenceOf(v reflect.Value) payloadReference {
	key := payloadReference{typ: v.Type(), ptr: v.Pointer()}
	switch v.Kind() { //nolint:exhaustive // Only pointers and slices need normalization.
	case reflect.Pointer:
		// A named pointer and its underlying pointer can refer to the same target.
		key.typ = reflect.PointerTo(v.Type().Elem())
	case reflect.Slice:
		key.len = v.Len()
	}
	return key
}

type payloadCloner struct {
	// An invalid value marks an original pointer retained as part of a map key.
	seen map[payloadReference]reflect.Value
	// A key found after its pointer was copied requires a new pass with all known keys retained.
	retry bool
}

func clonePayload[P any](payload P) P {
	cloner := payloadCloner{seen: make(map[payloadReference]reflect.Value)}
	result := payload
	cloner.cloneValue(reflect.ValueOf(&result).Elem())
	if cloner.retry {
		maps.DeleteFunc(cloner.seen, func(_ payloadReference, value reflect.Value) bool { return value.IsValid() })
		result = payload
		cloner.cloneValue(reflect.ValueOf(&result).Elem())
	}
	return result
}

func (c *payloadCloner) cloneValue(v reflect.Value) {
	switch v.Kind() { //nolint:exhaustive // Other kinds already hold the shallow value or handle.
	case reflect.Interface:
		if !v.IsNil() {
			cloned := reflect.New(v.Elem().Type()).Elem()
			cloned.Set(v.Elem())
			c.cloneValue(cloned)
			v.Set(cloned)
		}
	case reflect.Struct:
		for i := range v.NumField() { //nolint:modernize // Only the field values are needed.
			if field := v.Field(i); field.CanSet() {
				c.cloneValue(field)
			}
		}
	case reflect.Array:
		for i := range v.Len() {
			c.cloneValue(v.Index(i))
		}
	case reflect.Pointer, reflect.Slice, reflect.Map:
		if !v.IsNil() {
			c.cloneReference(v)
		}
	}
}

func (c *payloadCloner) cloneReference(v reflect.Value) {
	key := referenceOf(v)
	if cloned, ok := c.seen[key]; ok {
		if cloned.IsValid() {
			v.Set(cloned.Convert(v.Type()))
		}
		return
	}

	// Publish each clone before descending into it so a cycle points to the copy.
	switch v.Kind() { //nolint:exhaustive // Called only for pointers, slices and maps.
	case reflect.Pointer:
		cloned := reflect.New(v.Type().Elem()).Convert(v.Type())
		c.seen[key] = cloned
		cloned.Elem().Set(v.Elem())
		c.cloneValue(cloned.Elem())
		v.Set(cloned)
	case reflect.Slice:
		cloned := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		c.seen[key] = cloned
		reflect.Copy(cloned, v)
		for i := range cloned.Len() {
			c.cloneValue(cloned.Index(i))
		}
		v.Set(cloned)
	case reflect.Map:
		cloned := reflect.MakeMapWithSize(v.Type(), v.Len())
		c.seen[key] = cloned
		elem := reflect.New(v.Type().Elem()).Elem()
		for it := v.MapRange(); it.Next(); {
			k := it.Key()
			c.preserveKey(k)
			elem.SetIterValue(it)
			c.cloneValue(elem)
			cloned.SetMapIndex(k, elem)
		}
		v.Set(cloned)
	}
}

// preserveKey keeps pointers used in any comparable key, including when another
// field or map value refers to them. Map lookup depends on their Go identity.
func (c *payloadCloner) preserveKey(key reflect.Value) {
	switch key.Kind() { //nolint:exhaustive // Remaining comparable kinds have no pointer components to clone.
	case reflect.Pointer:
		if !key.IsNil() {
			ref := referenceOf(key)
			if c.seen[ref].IsValid() {
				c.retry = true
			}
			c.seen[ref] = reflect.Value{}
		}
	case reflect.Interface:
		c.preserveKey(key.Elem())
	case reflect.Struct:
		for i := range key.NumField() { //nolint:modernize // Only the field values are needed.
			c.preserveKey(key.Field(i))
		}
	case reflect.Array:
		for i := range key.Len() {
			c.preserveKey(key.Index(i))
		}
	}
}
