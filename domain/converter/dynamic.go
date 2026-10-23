// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package converter

import (
	"reflect"
	"strings"
)

// dynamicIdentity guards only the active recursion stack. Repeated siblings
// are copied independently; back edges with ignored descendants leave the
// destination unchanged. Unfiltered back edges are handled by the assignable
// leaf path before the guard and retain the source reference there.
type dynamicIdentity struct {
	typ reflect.Type
	ptr uintptr
}

// convertDynamicValue runs codecs once for a nested field or element, keeping
// the active recursion stack in the terminal rather than starting a new walk.
func (conv *Converter[T, U]) convertDynamicValue(path, name string, src, dst reflect.Value, active map[dynamicIdentity]bool) {
	if conv.skipValue(path, name, src) {
		return
	}
	// A root dynamic map key can collide with noIgnorePath; treat that value
	// as map-key context so error codecs never receive the internal sentinel.
	if path != noIgnorePath && conv.opts.codecsSet.HasErrorCodecs() {
		err := conv.opts.codecsSet.RunError(path, name, src, dst, func(p string, s, d reflect.Value) error {
			conv.convertDynamic(p, name, s, d, active)
			return nil
		})
		if err != nil {
			panic(codecError{err})
		}
		return
	}
	conv.opts.codecsSet.Run(name, src, dst, func(n string, s, d reflect.Value) {
		conv.convertDynamic(path, n, s, d, active)
	})
}

// convertDynamicLeaf applies built-in conversion without repeating codecs after
// unwrapping a value. The nil/zero options still apply to the concrete value.
func (conv *Converter[T, U]) convertDynamicLeaf(path, name string, src, dst reflect.Value) {
	if !conv.skipValue(path, name, src) {
		conv.convertAfterCodecs(path, name, src, dst)
	}
}

// convertDynamic walks values reached through an interface. Map string keys
// in this traversal are object paths, unlike statically typed map keys.
func (conv *Converter[T, U]) convertDynamic(path, name string, src, dst reflect.Value, active map[dynamicIdentity]bool) {
	if conv.isIgnoredPath(path) {
		return
	}
	if _, ignored := conv.opts.ignoreFields[name]; ignored {
		return
	}
	if src.Kind() == reflect.Interface {
		if src.IsNil() {
			if dst.Kind() == reflect.Interface {
				dst.SetZero()
			}
			return
		}
		src = src.Elem()
	}
	if dst.Kind() == reflect.Interface {
		out := reflect.New(src.Type()).Elem()
		if !dst.IsNil() && dst.Elem().Type() == src.Type() {
			out.Set(dst.Elem())
		}
		conv.convertDynamic(path, name, src, out, active)
		if out.Type().AssignableTo(dst.Type()) {
			dst.Set(out)
		}
		return
	}
	if src.Type() != dst.Type() || !conv.hasIgnorePathBelow(path) {
		conv.convertDynamicLeaf(path, name, src, dst)
		return
	}
	switch src.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice:
		if src.IsNil() {
			if !conv.opts.ignoreNilValues {
				dst.Set(src)
			}
			return
		}
		key := dynamicIdentity{src.Type(), src.Pointer()}
		if active[key] {
			return
		}
		active[key] = true
		defer delete(active, key)
	}
	switch src.Kind() {
	case reflect.Pointer:
		if conv.opts.sparseMerge && src.Elem().Kind() == reflect.Struct && isSparseStructEmpty(src.Elem()) {
			dst.SetZero()
			return
		}
		// Keep scalar pointers intact so zero checks apply to their presence,
		// not to the dereferenced value. Composite pointees still need walking.
		switch src.Elem().Kind() {
		case reflect.Pointer, reflect.Interface, reflect.Struct, reflect.Map, reflect.Slice, reflect.Array:
			// These pointees may contain ignored descendants.
		default:
			conv.convertDynamicLeaf(path, name, src, dst)
			detachPointer(dst)
			return
		}
		out := reflect.New(src.Type().Elem())
		if !dst.IsNil() {
			out.Elem().Set(dst.Elem())
		}
		conv.convertDynamic(path, name, src.Elem(), out.Elem(), active)
		dst.Set(out)
	case reflect.Struct:
		srcInfo := conv.typeCache.GetTypeInfo(src.Type())
		dstInfo := conv.typeCache.GetTypeInfo(dst.Type())
		old := reflect.New(dst.Type()).Elem()
		old.Set(dst)
		if !conv.opts.sparseMerge && !conv.opts.ignoreZeroValues && !conv.opts.ignoreNilValues {
			dst.Set(src)
			if conv.opts.copySlices {
				cloneSlicesInPlace(dst)
			}
			// Restore all exported fields before applying source mappings.
			for i := range dst.NumField() {
				if dstInfo.Fields[i].Exported {
					dst.Field(i).Set(old.Field(i))
				}
			}
		}
		for i := range src.NumField() {
			f := srcInfo.Fields[i]
			if !f.Exported {
				continue
			}
			fieldName := f.Lower
			target := i
			if mapped, ok := conv.opts.fieldMappings[fieldName]; ok {
				var exists bool
				target, exists = dstInfo.ByName[mapped]
				if !exists {
					continue
				}
			}
			if src.Field(i).Kind() == reflect.Pointer && src.Field(i).IsNil() {
				continue
			}
			conv.convertDynamicValue(conv.childPath(path, fieldName), fieldName, src.Field(i), dst.Field(target), active)
		}
	case reflect.Slice, reflect.Array:
		out := reflect.New(src.Type()).Elem()
		if src.Kind() == reflect.Slice {
			out = reflect.MakeSlice(src.Type(), src.Len(), src.Len())
		}
		for i := range src.Len() {
			if src.Kind() == reflect.Array && i < dst.Len() {
				out.Index(i).Set(dst.Index(i))
			}
			conv.convertDynamicValue(path, name, src.Index(i), out.Index(i), active)
		}
		dst.Set(out)
	case reflect.Map:
		out := reflect.MakeMapWithSize(src.Type(), src.Len())
		// Preserve ignored destination keys even if the source omits them.
		if src.Type().Key().Kind() == reflect.String {
			it := dst.MapRange()
			for it.Next() {
				if src.MapIndex(it.Key()).IsValid() {
					continue
				}
				keyName := strings.ToLower(it.Key().String())
				if conv.isIgnoredPath(conv.childPath(path, keyName)) {
					out.SetMapIndex(it.Key(), it.Value())
				}
			}
		}
		it := src.MapRange()
		for it.Next() {
			child := path
			childName := name
			if src.Type().Key().Kind() == reflect.String {
				childName = strings.ToLower(it.Key().String())
				child = conv.childPath(path, childName)
			}
			if conv.isIgnoredPath(child) {
				if old := dst.MapIndex(it.Key()); old.IsValid() {
					out.SetMapIndex(it.Key(), old)
				}
				continue
			}
			value := reflect.New(src.Type().Elem()).Elem()
			if old := dst.MapIndex(it.Key()); old.IsValid() {
				value.Set(old)
			}
			conv.convertDynamicValue(child, childName, it.Value(), value, active)
			out.SetMapIndex(it.Key(), value)
		}
		dst.Set(out)
	default:
		conv.convertDynamicLeaf(path, name, src, dst)
	}
}
