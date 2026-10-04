// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package toml

import (
	"encoding"
	"io"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"

	"github.com/altessa-s/go-atlas/config/loader/backend"
)

var (
	primitiveType       = reflect.TypeFor[toml.Primitive]()
	unmarshalerType     = reflect.TypeFor[toml.Unmarshaler]()
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
)

// DecodeKeys parses the TOML document and returns its root table. Children
// are bound by decoding them with BurntSushi/toml into shadow types whose
// values are [toml.Primitive]: struct fields mirror the destination's names,
// tags and embedding, so the decoder's own rules decide which field a key
// binds to — an exact name before a case-insensitive one, anonymous structs
// without a tag name flattened.
func (b *Backend) DecodeKeys(reader io.Reader) (backend.KeyNode, error) {
	var root toml.Primitive
	md, err := toml.NewDecoder(reader).Decode(&root)
	if err != nil {
		return nil, err
	}
	return &keyNode{md: &md, value: root}, nil
}

// keyNode is a value of a TOML document.
type keyNode struct {
	md    *toml.MetaData
	value toml.Primitive
}

// IsNull reports false: TOML has no null.
func (k *keyNode) IsNull() bool { return false }

// Fields binds a table to struct type t.
func (k *keyNode) Fields(t reflect.Type) (map[int]backend.KeyNode, bool) {
	if customDecoded(t) {
		return nil, false
	}
	sh := shadowOf(t)
	v := reflect.New(sh.typ)
	if err := k.md.PrimitiveDecode(k.value, v.Interface()); err != nil {
		return nil, false
	}
	return sh.collect(v.Elem(), k.md), true
}

// Entries binds a table to map type t.
func (k *keyNode) Entries(t reflect.Type) (map[any]backend.KeyNode, bool) {
	if customDecoded(t) {
		return nil, false
	}
	m := reflect.New(reflect.MapOf(t.Key(), primitiveType))
	if err := k.md.PrimitiveDecode(k.value, m.Interface()); err != nil {
		return nil, false
	}

	out := make(map[any]backend.KeyNode, m.Elem().Len())
	for it := m.Elem().MapRange(); it.Next(); {
		p, _ := it.Value().Interface().(toml.Primitive)
		out[it.Key().Interface()] = &keyNode{md: k.md, value: p}
	}
	return out, true
}

// Elems binds an array to slice or array type t.
func (k *keyNode) Elems(t reflect.Type) ([]backend.KeyNode, bool) {
	if customDecoded(t) {
		return nil, false
	}
	var s []toml.Primitive
	if err := k.md.PrimitiveDecode(k.value, &s); err != nil {
		return nil, false
	}

	out := make([]backend.KeyNode, len(s))
	for i, p := range s {
		out[i] = &keyNode{md: k.md, value: p}
	}
	return out, true
}

// customDecoded reports whether the decoder hands values of type t to t's own
// unmarshaler, so their keys bind to nothing the loader can see.
func customDecoded(t reflect.Type) bool {
	pt := reflect.PointerTo(t)
	return t.Implements(unmarshalerType) || t.Implements(textUnmarshalerType) ||
		pt.Implements(unmarshalerType) || pt.Implements(textUnmarshalerType)
}

// shadow is a struct type mirroring a destination struct for binding: every
// field the decoder can fill becomes a toml.Primitive, and every anonymous
// struct it flattens becomes an embedded shadow.
type shadow struct {
	typ    reflect.Type
	fields []shadowField
}

// shadowField maps a field of the shadow type to the destination field.
type shadowField struct {
	index    int     // field index in the destination struct
	embedded *shadow // the flattened anonymous struct, or nil for a value
}

// collect returns the destination fields the decoded shadow value v sets.
func (s *shadow) collect(v reflect.Value, md *toml.MetaData) map[int]backend.KeyNode {
	out := make(map[int]backend.KeyNode, len(s.fields))
	for j, f := range s.fields {
		fv := v.Field(j)
		if f.embedded != nil {
			out[f.index] = &fieldsNode{fields: f.embedded.collect(fv, md)}
			continue
		}
		if !fv.IsZero() {
			p, _ := fv.Interface().(toml.Primitive)
			out[f.index] = &keyNode{md: md, value: p}
		}
	}
	return out
}

var shadowCache sync.Map // map[reflect.Type]*shadow

// shadowOf returns the shadow of struct type t, cached per type so the
// decoder's per-type field analysis treats repeated embeddings alike.
func shadowOf(t reflect.Type) *shadow {
	if s, ok := shadowCache.Load(t); ok {
		sh, _ := s.(*shadow)
		return sh
	}
	sh := buildShadow(t, map[reflect.Type]bool{})
	shadowCache.Store(t, sh)
	return sh
}

// buildShadow mirrors the field selection of BurntSushi/toml's typeFields:
// unexported fields other than embedded structs are skipped, "-" excludes a
// field, and an anonymous struct (or pointer to one) without a tag name is
// flattened. Names and tag names are kept, so the decoder applies its own
// exact-then-case-insensitive matching and embedding dominance rules.
func buildShadow(t reflect.Type, visiting map[reflect.Type]bool) *shadow {
	if s, ok := shadowCache.Load(t); ok {
		sh, _ := s.(*shadow)
		return sh
	}
	visiting[t] = true
	defer delete(visiting, t)

	sh := &shadow{}
	sfs := make([]reflect.StructField, 0, t.NumField())
	for i := range t.NumField() {
		sf := t.Field(i)
		tag := sf.Tag.Get("toml")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")

		ft := sf.Type
		if ft.Name() == "" && ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		if name == "" && sf.Anonymous && ft.Kind() == reflect.Struct {
			if visiting[ft] {
				continue // the decoder explores each embedded type once
			}
			inner := buildShadow(ft, visiting)
			sfs = append(sfs, reflect.StructField{
				Name:      "Embedded_" + strconv.Itoa(i),
				Type:      inner.typ,
				Anonymous: true,
			})
			sh.fields = append(sh.fields, shadowField{index: i, embedded: inner})
			continue
		}
		if !sf.IsExported() {
			continue // the decoder cannot write unexported fields
		}

		f := reflect.StructField{Name: sf.Name, Type: primitiveType}
		if name != "" {
			f.Tag = reflect.StructTag(`toml:` + strconv.Quote(name))
		}
		sfs = append(sfs, f)
		sh.fields = append(sh.fields, shadowField{index: i})
	}
	sh.typ = reflect.StructOf(sfs)
	return sh
}

// fieldsNode holds the fields of an anonymous struct the decoder flattens into
// the enclosing table.
type fieldsNode struct {
	fields map[int]backend.KeyNode
}

// IsNull reports false: an embedded struct is never null.
func (n *fieldsNode) IsNull() bool { return false }

// Fields returns the embedded struct's fields.
func (n *fieldsNode) Fields(reflect.Type) (map[int]backend.KeyNode, bool) { return n.fields, true }

// Entries reports false: an embedded struct is not a map.
func (n *fieldsNode) Entries(reflect.Type) (map[any]backend.KeyNode, bool) { return nil, false }

// Elems reports false: an embedded struct is not an array.
func (n *fieldsNode) Elems(reflect.Type) ([]backend.KeyNode, bool) { return nil, false }

// Ensure Backend implements backend.KeyDecoder.
var _ backend.KeyDecoder = (*Backend)(nil)
