// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"fmt"
	"net/netip"
	"reflect"
	"strings"
	"time"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
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
	// Fast paths for the common payloads, with the same result as
	// reflection. They overwrite value, this call's own copy of the argument.
	switch p := any(&value).(type) {
	case *string:
		*p = strings.Clone(*p)
		return value
	case *[]byte:
		if *p != nil {
			*p = append(make([]byte, 0, len(*p)), *p...)
		}
		return value
	}
	return cloneReflect(value)
}

// cloneReflect is cloneSecret's reflection path. It is a separate function so
// that taking value's address for reflection does not move cloneSecret's
// argument to the heap on the fast paths.
func cloneReflect[T any](value T) T {
	var out T
	reflect.ValueOf(&out).Elem().Set((&deepCopier{}).copy(reflect.ValueOf(&value).Elem()))
	return out
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

// immutableTypes are standard library value types whose unexported reference
// fields point at immutable data, so sharing them between copies is safe.
var immutableTypes = coremaps.NewImmutableMap(map[reflect.Type]struct{}{
	reflect.TypeFor[time.Time]():      {},
	reflect.TypeFor[time.Location]():  {},
	reflect.TypeFor[netip.Addr]():     {},
	reflect.TypeFor[netip.Prefix]():   {},
	reflect.TypeFor[netip.AddrPort](): {},
})

// checkCloneable reports, wrapping [ErrUncloneablePayload], the first place
// where the reflection copy of payload type t would share mutable memory with
// the original: an unexported field that can reach a reference, or a
// channel, func or unsafe pointer, not covered by a Clone() T method.
// Interface values are copied by their dynamic type and are not checked.
func checkCloneable(t reflect.Type) error {
	if path, ok := shallowPath(t, t.String(), map[reflect.Type]bool{}); ok {
		return fmt.Errorf("%w: %s", ErrUncloneablePayload, path)
	}
	return nil
}

// hasCloneMethod reports whether t has a Clone method of shape func() t.
// Interface types never count: deepCopier follows an interface to its dynamic
// value, and an interface method's reflected type has no receiver parameter,
// so the shape check below would misread it.
func hasCloneMethod(t reflect.Type) bool {
	if t.Kind() == reflect.Interface {
		return false
	}
	m, ok := t.MethodByName("Clone")
	return ok && m.Type.NumIn() == 1 && m.Type.NumOut() == 1 && m.Type.Out(0) == t
}

// shallowPath returns the path of the first shared-memory location reachable
// from t, mirroring deepCopier.copy.
func shallowPath(t reflect.Type, path string, seen map[reflect.Type]bool) (string, bool) {
	if seen[t] || hasCloneMethod(t) {
		return "", false
	}
	if immutableTypes.Contains(t) {
		return "", false
	}
	seen[t] = true
	switch t.Kind() {
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return path + " (" + t.Kind().String() + ")", true
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return shallowPath(t.Elem(), path+"[elem]", seen)
	case reflect.Map:
		if p, ok := shallowPath(t.Key(), path+"[key]", seen); ok {
			return p, true
		}
		return shallowPath(t.Elem(), path+"[value]", seen)
	case reflect.Struct:
		for f := range t.Fields() {
			fp := path + "." + f.Name
			if f.IsExported() {
				if p, ok := shallowPath(f.Type, fp, seen); ok {
					return p, true
				}
				continue
			}
			if reachesReference(f.Type, map[reflect.Type]bool{}) {
				return fp + " (unexported " + f.Type.String() + ")", true
			}
		}
	default:
		// Scalars and strings are copied; interfaces by their dynamic type.
	}
	return "", false
}

// reachesReference reports whether a value of type t, copied by assignment,
// can share memory: it is or contains a pointer, map, slice, interface,
// channel, func or unsafe pointer, other than through an immutable type.
func reachesReference(t reflect.Type, seen map[reflect.Type]bool) bool {
	if seen[t] {
		return false
	}
	if immutableTypes.Contains(t) {
		return false
	}
	seen[t] = true
	switch t.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface, reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return true
	case reflect.Array:
		return reachesReference(t.Elem(), seen)
	case reflect.Struct:
		for f := range t.Fields() {
			if reachesReference(f.Type, seen) {
				return true
			}
		}
	default:
		// Scalars and strings hold no reference.
	}
	return false
}

// reachesInterface reports whether the deep copy of t can reach an interface
// value, whose dynamic type checkCloneable cannot see; such payloads are
// checked value by value with checkCloneableValue.
func reachesInterface(t reflect.Type, seen map[reflect.Type]bool) bool {
	if seen[t] || hasCloneMethod(t) || immutableTypes.Contains(t) {
		return false
	}
	seen[t] = true
	switch t.Kind() {
	case reflect.Interface:
		return true
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return reachesInterface(t.Elem(), seen)
	case reflect.Map:
		return reachesInterface(t.Key(), seen) || reachesInterface(t.Elem(), seen)
	case reflect.Struct:
		for f := range t.Fields() {
			if f.IsExported() && reachesInterface(f.Type, seen) {
				return true
			}
		}
	default:
		// Other kinds hold no interface the copy follows.
	}
	return false
}

// checkCloneableValue is checkCloneable for one payload value: it follows
// interface values to their dynamic types, as deepCopier does, and reports,
// wrapping [ErrUncloneablePayload], the first location whose copy would share
// mutable memory with the original.
func checkCloneableValue(v reflect.Value) error {
	if path, ok := shallowValuePath(v, v.Type().String(), map[copyKey]bool{}); ok {
		return fmt.Errorf("%w: %s", ErrUncloneablePayload, path)
	}
	return nil
}

// shallowValuePath mirrors deepCopier.copy over a value.
// References are identified like deepCopier identifies them (refKey: type,
// address and, for slices, length and capacity), so cycles terminate and a
// pointer sharing an address with a different-typed one is still visited.
func shallowValuePath(v reflect.Value, path string, seen map[copyKey]bool) (string, bool) {
	if !v.IsValid() {
		return "", false
	}
	t := v.Type()
	if v.Kind() != reflect.Interface && (hasCloneMethod(t) || immutableTypes.Contains(t)) {
		return "", false
	}
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return "", false
		}
		return shallowValuePath(v.Elem(), path+"("+v.Elem().Type().String()+")", seen)
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		if v.IsNil() {
			return "", false
		}
		return path + " (" + v.Kind().String() + ")", true
	case reflect.Pointer, reflect.Map, reflect.Slice:
		if v.IsNil() {
			return "", false
		}
		k, _ := refKey(v)
		if seen[k] {
			return "", false
		}
		seen[k] = true
		switch v.Kind() {
		case reflect.Pointer:
			return shallowValuePath(v.Elem(), path+"[elem]", seen)
		case reflect.Map:
			for it := v.MapRange(); it.Next(); {
				if p, ok := shallowValuePath(it.Key(), path+"[key]", seen); ok {
					return p, true
				}
				if p, ok := shallowValuePath(it.Value(), path+"[value]", seen); ok {
					return p, true
				}
			}
			return "", false
		default:
			return shallowElems(v, path, seen)
		}
	case reflect.Array:
		return shallowElems(v, path, seen)
	case reflect.Struct:
		for f, fv := range v.Fields() {
			fp := path + "." + f.Name
			if f.IsExported() {
				if p, ok := shallowValuePath(fv, fp, seen); ok {
					return p, true
				}
				continue
			}
			if reachesReference(f.Type, map[reflect.Type]bool{}) {
				return fp + " (unexported " + f.Type.String() + ")", true
			}
		}
	default:
		// Scalars and strings are copied.
	}
	return "", false
}

// shallowElems checks the elements of a slice or array value.
func shallowElems(v reflect.Value, path string, seen map[copyKey]bool) (string, bool) {
	if v.Type().Elem().Kind() == reflect.Uint8 {
		return "", false
	}
	for i := range v.Len() {
		if p, ok := shallowValuePath(v.Index(i), path+"[elem]", seen); ok {
			return p, true
		}
	}
	return "", false
}
