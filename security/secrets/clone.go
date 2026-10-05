// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"reflect"
	"strings"
)

// cloneSecret returns a deep copy of a secret payload, so a copy can be
// modified or cleared without affecting the original and vice versa.
//
// A type with an exported method Clone() returning its own type controls its
// own copy wherever it occurs in the payload (top level, inside an interface,
// an exported field, a map or a slice). Everything else is copied by
// reflection: strings, slices, arrays, maps (keys and values), pointers,
// interfaces and exported struct fields are copied recursively. Identical references reached twice (the same pointer, map or
// slice header) map to the same copy, so shared and cyclic structures keep
// their shape and copying terminates. Overlapping slice views and interior
// pointers become independent copies, and unexported struct fields,
// channels, funcs and unsafe pointers cannot be copied by reflection and are
// shared by assignment: payload types relying on those should implement
// such a Clone method.
func cloneSecret[T any](value T) T {
	rv := reflect.ValueOf(&value).Elem()
	out := reflect.New(rv.Type())
	out.Elem().Set((&deepCopier{}).copy(rv))
	return *out.Interface().(*T) //nolint:errcheck // out is a *T by construction
}

// copyKey identifies a reference already copied: its unnamed type (so a
// defined pointer, map or slice type and its underlying type share one
// copy) and address, plus length and capacity for slice headers.
type copyKey struct {
	typ      reflect.Type
	ptr      uintptr
	len, cap int
}

// deepCopier performs one deep copy, remembering the references it copied.
type deepCopier struct {
	seen map[copyKey]reflect.Value
}

// lookup returns the copy of the reference identified by k, converted to t.
func (d *deepCopier) lookup(k copyKey, t reflect.Type) (reflect.Value, bool) {
	v, ok := d.seen[k]
	if !ok {
		return reflect.Value{}, false
	}
	return v.Convert(t), true
}

func (d *deepCopier) remember(k copyKey, v reflect.Value) {
	if d.seen == nil {
		d.seen = make(map[copyKey]reflect.Value)
	}
	d.seen[k] = v
}

// refKey returns the identity of a non-nil pointer, map or slice v.
func refKey(v reflect.Value) (copyKey, bool) {
	if !v.IsValid() {
		return copyKey{}, false
	}
	t := v.Type()
	switch k := v.Kind(); {
	case k == reflect.Pointer && !v.IsNil():
		return copyKey{typ: reflect.PointerTo(t.Elem()), ptr: v.Pointer()}, true
	case k == reflect.Map && !v.IsNil():
		return copyKey{typ: reflect.MapOf(t.Key(), t.Elem()), ptr: v.Pointer()}, true
	case k == reflect.Slice && !v.IsNil():
		return copyKey{typ: reflect.SliceOf(t.Elem()), ptr: v.Pointer(), len: v.Len(), cap: v.Cap()}, true
	default:
		return copyKey{}, false
	}
}

// cloneMethod returns v's Clone method when it has the shape func() T for
// v's own type T, and v is not a nil pointer, map, slice or interface.
func cloneMethod(v reflect.Value) (reflect.Value, bool) {
	if k := v.Kind(); (k == reflect.Pointer || k == reflect.Map || k == reflect.Slice || k == reflect.Interface) && v.IsNil() {
		return reflect.Value{}, false
	}
	mt, ok := v.Type().MethodByName("Clone")
	if !ok || mt.Type.NumIn() != 1 || mt.Type.NumOut() != 1 || mt.Type.Out(0) != v.Type() {
		return reflect.Value{}, false
	}
	return v.Method(mt.Index), true
}

// copy returns a deep copy of v with v's type.
func (d *deepCopier) copy(v reflect.Value) reflect.Value {
	t := v.Type()
	if v.Kind() != reflect.Interface {
		if clone, ok := cloneMethod(v); ok {
			k, isRef := refKey(v)
			if isRef {
				if c, ok := d.lookup(k, t); ok {
					return c
				}
			}
			out := clone.Call(nil)[0]
			if isRef {
				d.remember(k, out)
			}
			return out
		}
	}
	switch v.Kind() {
	case reflect.String:
		return reflect.ValueOf(strings.Clone(v.String())).Convert(t)

	case reflect.Interface:
		if v.IsNil() {
			return reflect.Zero(t)
		}
		out := reflect.New(t).Elem()
		out.Set(d.copy(v.Elem()))
		return out

	case reflect.Pointer:
		if v.IsNil() {
			return reflect.Zero(t)
		}
		k, _ := refKey(v)
		if c, ok := d.lookup(k, t); ok {
			return c
		}
		elem := reflect.New(t.Elem())
		out := elem.Convert(t) // keeps a defined pointer type
		d.remember(k, out)
		elem.Elem().Set(d.copy(v.Elem()))
		return out

	case reflect.Map:
		if v.IsNil() {
			return reflect.Zero(t)
		}
		k, _ := refKey(v)
		if c, ok := d.lookup(k, t); ok {
			return c
		}
		out := reflect.MakeMapWithSize(t, v.Len())
		d.remember(k, out)
		for it := v.MapRange(); it.Next(); {
			out.SetMapIndex(d.copy(it.Key()), d.copy(it.Value()))
		}
		return out

	case reflect.Slice:
		if v.IsNil() {
			return reflect.Zero(t)
		}
		k, _ := refKey(v)
		if c, ok := d.lookup(k, t); ok {
			return c
		}
		out := reflect.MakeSlice(t, v.Len(), v.Len())
		d.remember(k, out)
		if t.Elem().Kind() == reflect.Uint8 {
			reflect.Copy(out, v)
			return out
		}
		for i := range v.Len() {
			out.Index(i).Set(d.copy(v.Index(i)))
		}
		return out

	case reflect.Array:
		out := reflect.New(t).Elem()
		for i := range v.Len() {
			out.Index(i).Set(d.copy(v.Index(i)))
		}
		return out

	case reflect.Struct:
		out := reflect.New(t).Elem()
		out.Set(v)
		for i := range t.NumField() {
			if t.Field(i).IsExported() {
				out.Field(i).Set(d.copy(v.Field(i)))
			}
		}
		return out

	default:
		// Numbers, booleans, channels, funcs and unsafe pointers: copied by
		// assignment.
		return v
	}
}
