// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldtracker

import (
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas-core/encoding/keys"
)

// formatMapKey escapes path syntax after encoding the identity of a key.
func formatMapKey(v reflect.Value) string {
	key := mapKeyIdentity(v)
	if strings.ContainsAny(key, "[]\\") {
		key = mapPathEscapes.Replace(key)
	}
	return key
}

var (
	mapPathEscapes   = strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]")
	fieldPathEscapes = strings.NewReplacer("\\", "\\\\", ".", "\\.", "[", "\\[", "]", "\\]")
)

func escapeFieldSegment(name string) string {
	if strings.ContainsAny(name, ".[]\\") {
		return fieldPathEscapes.Replace(name)
	}
	return name
}

func mapKeyIdentity(v reflect.Value) string {
	switch v.Kind() { //nolint:exhaustive // Map keys can only use comparable types.
	case reflect.Interface:
		if v.IsNil() {
			return "@nil"
		}
		key := v.Elem()
		if key.Type() == reflect.TypeFor[string]() {
			text := key.String()
			if strings.HasPrefix(text, "@") {
				return "@" + text
			}
			return text
		}
		typ := key.Type()
		if name := qualifiedName(typ); name != "" && !strings.Contains(name, ":") {
			return "@" + name + ":" + mapKeyIdentity(key)
		}
		return "@:" + keys.Compose(typeIdentity(typ), mapKeyIdentity(key))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(canonicalZero(v.Float()), 'f', -1, v.Type().Bits())
	case reflect.Complex64, reflect.Complex128:
		n := v.Complex()
		return strconv.FormatComplex(complex(canonicalZero(real(n)), canonicalZero(imag(n))), 'g', -1, v.Type().Bits())
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.String:
		return v.String()
	case reflect.Pointer, reflect.Chan, reflect.UnsafePointer:
		return "0x" + strconv.FormatUint(uint64(v.Pointer()), 16)
	case reflect.Array:
		return compositeKey(v.Len(), v.Index)
	case reflect.Struct:
		return compositeKey(v.NumField(), v.Field)
	}
	panic("fieldtracker: non-comparable map key")
}

// Equal Go map keys must have the same path, including both signs of zero.
func canonicalZero(n float64) float64 {
	if n == 0 {
		return 0
	}
	return n
}

func compositeKey(length int, value func(int) reflect.Value) string {
	parts := make([]string, length)
	for i := range parts {
		parts[i] = mapKeyIdentity(value(i))
	}
	return keys.Compose(parts...)
}

// Go permits distinct named types with identical package and type names in
// different lexical scopes. Keep their printed names distinct across trackers
// so a path returned by one tracker can be used as an ignore rule on another.
var namedKeyTypes = struct {
	sync.Mutex
	names map[reflect.Type]string
	count map[string]int
}{names: make(map[reflect.Type]string), count: make(map[string]int)}

func qualifiedName(typ reflect.Type) string {
	name := typ.Name()
	if name == "" || typ.PkgPath() == "" {
		return name
	}
	namedKeyTypes.Lock()
	defer namedKeyTypes.Unlock()
	if cached, ok := namedKeyTypes.names[typ]; ok {
		return cached
	}
	name = typ.PkgPath() + "." + name
	namedKeyTypes.count[name]++
	if count := namedKeyTypes.count[name]; count > 1 {
		name += "#" + strconv.Itoa(count)
	}
	namedKeyTypes.names[typ] = name
	return name
}

// typeIdentity describes an unnamed type using the same constituents Go
// uses for type identity. Reflect.Type.String elides nested package paths.
func typeIdentity(typ reflect.Type) string {
	if name := qualifiedName(typ); name != "" {
		return keys.Compose("named", name)
	}
	kind := typ.Kind().String()
	switch typ.Kind() { //nolint:exhaustive // Only composite types are unnamed.
	case reflect.Pointer, reflect.Slice:
		return keys.Compose(kind, typeIdentity(typ.Elem()))
	case reflect.Array:
		return keys.Compose(kind, strconv.Itoa(typ.Len()), typeIdentity(typ.Elem()))
	case reflect.Chan:
		return keys.Compose(kind, typ.ChanDir().String(), typeIdentity(typ.Elem()))
	case reflect.Map:
		return keys.Compose(kind, typeIdentity(typ.Key()), typeIdentity(typ.Elem()))
	case reflect.Struct:
		parts := []string{kind}
		for field := range typ.Fields() {
			parts = append(parts, keys.Compose(field.Name, field.PkgPath, string(field.Tag), strconv.FormatBool(field.Anonymous), typeIdentity(field.Type)))
		}
		return keys.Compose(parts...)
	case reflect.Interface:
		parts := []string{kind}
		for method := range typ.Methods() {
			parts = append(parts, keys.Compose(method.Name, method.PkgPath, typeIdentity(method.Type)))
		}
		return keys.Compose(parts...)
	case reflect.Func:
		parts := []string{kind, strconv.FormatBool(typ.IsVariadic()), strconv.Itoa(typ.NumIn())}
		for in := range typ.Ins() {
			parts = append(parts, typeIdentity(in))
		}
		for out := range typ.Outs() {
			parts = append(parts, typeIdentity(out))
		}
		return keys.Compose(parts...)
	}
	panic("fieldtracker: unexpected unnamed type")
}
