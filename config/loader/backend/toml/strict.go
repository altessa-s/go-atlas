// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package toml

import (
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/altessa-s/go-atlas/config/loader/backend"
)

// DecodeStrict decodes the TOML data from the reader like Decode, but rejects
// keys that bind to no field of a destination struct. The returned error
// wraps [backend.ErrUnknownField] and names every such key.
//
// BurntSushi/toml reports as undecoded every key it did not decode into a
// concrete type, which also covers the content of maps of interfaces,
// interfaces, [toml.Primitive] values and types with their own unmarshaler.
// Their content is free-form, so only keys that reach a struct
// without a matching field are unknown.
func (b *Backend) DecodeStrict(reader io.Reader, in any) error {
	md, err := toml.NewDecoder(reader).Decode(in)
	if err != nil {
		return err
	}

	var unknown []toml.Key
	root := reflect.TypeOf(in)
	for _, key := range md.Undecoded() {
		// Report an unknown table once, not every key beneath it.
		if slices.ContainsFunc(unknown, func(u toml.Key) bool { return hasPrefix(key, u) }) {
			continue
		}
		if unknownKey(root, key) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		names := make([]string, len(unknown))
		for i, key := range unknown {
			names[i] = key.String()
		}
		return fmt.Errorf("%w: %s in type %s", backend.ErrUnknownField, strings.Join(names, ", "), indirectType(root))
	}
	return nil
}

// hasPrefix reports whether key lies within prefix.
func hasPrefix(key, prefix toml.Key) bool {
	return len(key) >= len(prefix) && slices.Equal(key[:len(prefix)], prefix)
}

// unknownKey reports whether key, followed from destination type t, names a
// struct field that does not exist. It reports false when the key leads into
// a destination accepting arbitrary content, or binds all the way down.
func unknownKey(t reflect.Type, key toml.Key) bool {
	for i := 0; i < len(key); {
		t = indirectType(t)
		if t == primitiveType || customDecoded(t) {
			return false
		}
		switch t.Kind() {
		case reflect.Struct:
			ft, ok := fieldByKey(t, key[i])
			if !ok {
				return true
			}
			if ft == nil {
				return false
			}
			t = ft
			i++
		case reflect.Map:
			t = t.Elem()
			i++
		case reflect.Slice, reflect.Array:
			// An array of tables adds no key part per element.
			t = t.Elem()
		default:
			// An interface, or a value the decoder rejects on its own.
			return false
		}
	}
	return false
}

// fieldByKey returns the type of the field of struct t that the decoder binds
// key to, or false when it binds to none. It asks the decoder itself, by
// decoding the key into the shadow of t, so field dominance, ambiguity and
// case-insensitive matching are BurntSushi/toml's own. A key the probe cannot
// express yields a nil type and true: it is not reported.
func fieldByKey(t reflect.Type, key string) (reflect.Type, bool) {
	sh := shadowOf(t)
	v := reflect.New(sh.typ)
	if _, err := toml.Decode(quoteKey(key)+" = 0\n", v.Interface()); err != nil {
		return nil, true
	}
	return sh.boundField(t, v.Elem())
}

// quoteKey returns key as a TOML basic string: unlike strconv.Quote it
// escapes control characters only in forms TOML accepts.
func quoteKey(key string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for _, r := range key {
		switch {
		case r == '"' || r == '\\':
			sb.WriteByte('\\')
			sb.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&sb, `\u%04X`, r)
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// boundField returns the type of the field of struct t whose shadow field is
// set in the decoded shadow value v.
func (s *shadow) boundField(t reflect.Type, v reflect.Value) (reflect.Type, bool) {
	for _, f := range s.fields {
		fv := v.Field(f.shadow)
		ft := t.Field(f.index).Type
		if f.embedded != nil {
			if bt, ok := f.embedded.boundField(indirectType(ft), fv); ok {
				return bt, true
			}
			continue
		}
		if !fv.IsZero() {
			return ft, true
		}
	}
	return nil, false
}

// indirectType returns the type t points to through any number of pointers.
func indirectType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// Ensure Backend implements backend.StrictDecoder.
var _ backend.StrictDecoder = (*Backend)(nil)
