// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader

import (
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/altessa-s/go-atlas/config/loader/backend"
)

// presence records which values of the configuration the files and the
// environment set explicitly, so default tags and Default() leave an explicit
// false, 0 or "" alone. It is bound to the configuration type: struct fields
// by index (an anonymous or inline field holds the fields of its struct), map
// entries by decoded key and sequence elements by decoded position. A nil
// *presence is an absent value.
type presence struct {
	fields  map[int]*presence
	entries map[any]*presence
	elems   []*presence
	// capacity is the length of the slice allocated for the elements, which a
	// decoder that reuses slices fills in place when a later file is shorter.
	capacity int
	// set reports that a file key or an environment variable wrote the value.
	set bool
	// null reports that the file wrote an explicit null.
	null bool
}

// isSet reports whether the value was written explicitly.
func (p *presence) isSet() bool { return p != nil && p.set }

// isNull reports whether a file set the value to null.
func (p *presence) isNull() bool { return p != nil && p.null }

// field returns the presence of struct field i.
func (p *presence) field(i int) *presence {
	if p == nil {
		return nil
	}
	return p.fields[i]
}

// entry returns the presence of the map entry with key k.
func (p *presence) entry(k reflect.Value) *presence {
	if p == nil || p.entries == nil || !k.CanInterface() {
		return nil
	}
	return p.entries[k.Interface()]
}

// elem returns the presence of sequence element i.
func (p *presence) elem(i int) *presence {
	if p == nil || i >= len(p.elems) {
		return nil
	}
	return p.elems[i]
}

// at returns the presence of the field at the struct index path.
func (p *presence) at(path []int) *presence {
	for _, i := range path {
		p = p.field(i)
	}
	return p
}

// bindPresence binds a decoded document value to type t.
func bindPresence(t reflect.Type, n backend.KeyNode) *presence {
	if n == nil {
		return nil
	}
	p := &presence{set: true}
	if n.IsNull() {
		p.null = true
		return p
	}

	t = derefType(t)
	switch t.Kind() {
	case reflect.Struct:
		if fs, ok := n.Fields(t); ok {
			p.fields = make(map[int]*presence, len(fs))
			for i, c := range fs {
				p.fields[i] = bindPresence(t.Field(i).Type, c)
			}
		}
	case reflect.Map:
		if es, ok := n.Entries(t); ok {
			p.entries = make(map[any]*presence, len(es))
			for k, c := range es {
				p.entries[k] = bindPresence(t.Elem(), c)
			}
		}
	case reflect.Slice, reflect.Array:
		if es, ok := n.Elems(t); ok {
			p.elems = make([]*presence, len(es))
			for i, c := range es {
				p.elems[i] = bindPresence(t.Elem(), c)
			}
			p.capacity = len(es)
		}
	default:
	}
	return p
}

// mergePresence overlays src, bound from a later file, onto dst the way the
// decoder writes the later file over the configuration of type t: struct
// fields merge one by one, a map entry is replaced wholesale, a slice is
// replaced unless reuseSlices — BurntSushi/toml fills an existing slice with
// enough capacity element by element — and a null resets pointers, maps,
// slices and interfaces but leaves other values, and existing map entries, as
// they were.
func mergePresence(dst, src *presence, t reflect.Type, reuseSlices bool) *presence {
	switch {
	case src == nil:
		return dst
	case dst == nil:
		return src
	case src.null:
		if nullable(t.Kind()) {
			return src
		}
		return dst
	case dst.null:
		return src
	}

	t = derefType(t)
	out := &presence{set: true, capacity: src.capacity}
	switch t.Kind() {
	case reflect.Struct:
		if src.fields == nil {
			return src
		}
		out.fields = maps.Clone(dst.fields)
		if out.fields == nil {
			out.fields = make(map[int]*presence, len(src.fields))
		}
		for i, c := range src.fields {
			out.fields[i] = mergePresence(out.fields[i], c, t.Field(i).Type, reuseSlices)
		}
	case reflect.Map:
		if src.entries == nil {
			return src
		}
		out.entries = maps.Clone(dst.entries)
		if out.entries == nil {
			out.entries = make(map[any]*presence, len(src.entries))
		}
		for k, c := range src.entries {
			if _, ok := out.entries[k]; ok && c.isNull() {
				continue // a null does not override an existing entry
			}
			out.entries[k] = c
		}
	case reflect.Slice:
		if !reuseSlices || src.elems == nil || len(src.elems) > dst.capacity {
			return src
		}
		out.capacity = dst.capacity
		out.elems = make([]*presence, len(src.elems))
		for i, c := range src.elems {
			out.elems[i] = mergePresence(dst.elem(i), c, t.Elem(), reuseSlices)
		}
	case reflect.Array:
		// Arrays are written in place: elements past the later sequence keep
		// their values.
		out.elems = slices.Clone(dst.elems)
		for i, c := range src.elems {
			if i >= len(out.elems) {
				out.elems = append(out.elems, c)
				continue
			}
			if reuseSlices {
				c = mergePresence(out.elems[i], c, t.Elem(), reuseSlices)
			}
			out.elems[i] = c
		}
	default:
		return src
	}
	return out
}

// nullable reports whether decoding a null resets a value of kind k.
func nullable(k reflect.Kind) bool {
	switch k {
	case reflect.Interface, reflect.Pointer, reflect.Map, reflect.Slice:
		return true
	default:
		return false
	}
}

// derefType strips all pointer levels from t.
func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// stepKind tells what a pathStep selects.
type stepKind uint8

const (
	fieldStep stepKind = iota // a struct field by index
	keyStep                   // a map entry by key
	elemStep                  // a slice or array element by index
)

// pathStep is one step from a value to a value it contains.
type pathStep struct {
	key   any
	index int
	kind  stepKind
}

// indexSteps converts a struct index path to path steps.
func indexSteps(base []pathStep, index []int) []pathStep {
	steps := slices.Grow(slices.Clip(base), len(index))
	for _, i := range index {
		steps = append(steps, pathStep{kind: fieldStep, index: i})
	}
	return steps
}

// withStep returns base extended by one step without aliasing base.
func withStep(base []pathStep, step pathStep) []pathStep {
	return append(slices.Clip(base), step)
}

// mark records that the environment wrote the value at steps. Values along
// the way exist from then on, so an explicit null from a file no longer holds
// for them.
func (p *presence) mark(steps []pathStep) {
	cur := p
	for _, s := range steps {
		cur.null = false
		cur = cur.child(s)
	}
	cur.null = false
	cur.set = true
}

// child returns the presence s selects, creating it when absent.
func (p *presence) child(s pathStep) *presence {
	var c *presence
	switch s.kind {
	case fieldStep:
		if p.fields == nil {
			p.fields = make(map[int]*presence)
		}
		if c = p.fields[s.index]; c == nil {
			c = &presence{}
			p.fields[s.index] = c
		}
	case keyStep:
		if p.entries == nil {
			p.entries = make(map[any]*presence)
		}
		if c = p.entries[s.key]; c == nil {
			c = &presence{}
			p.entries[s.key] = c
		}
	case elemStep:
		if s.index >= len(p.elems) {
			p.elems = append(p.elems, make([]*presence, s.index+1-len(p.elems))...)
		}
		if c = p.elems[s.index]; c == nil {
			c = &presence{}
			p.elems[s.index] = c
		}
	}
	return c
}

// genericNode adapts the generic decode of a document — maps, slices and
// scalars — to backend.KeyNode for a backend that does not implement
// backend.KeyDecoder. Struct fields bind by tag name or field name, an exact
// match before a case-insensitive one; anonymous structs without a tag name
// are flattened; only string map keys are recognized.
type genericNode struct {
	value any
	tag   string
}

// IsNull reports whether the value is null.
func (n genericNode) IsNull() bool { return n.value == nil }

// Fields binds a mapping to struct type t.
func (n genericNode) Fields(t reflect.Type) (map[int]backend.KeyNode, bool) {
	m, ok := genericMap(n.value)
	if !ok {
		return nil, false
	}
	out := make(map[int]backend.KeyNode)
	for i := range t.NumField() {
		sf := t.Field(i)
		name, _, _ := strings.Cut(sf.Tag.Get(n.tag), ",")
		if name == "-" || sf.PkgPath != "" && !sf.Anonymous {
			continue
		}
		if name == "" && sf.Anonymous && derefType(sf.Type).Kind() == reflect.Struct {
			out[i] = n
			continue
		}
		if name == "" {
			name = sf.Name
		}
		if v, ok := m[name]; ok {
			out[i] = genericNode{value: v, tag: n.tag}
			continue
		}
		for _, k := range slices.Sorted(maps.Keys(m)) {
			if strings.EqualFold(k, name) {
				out[i] = genericNode{value: m[k], tag: n.tag}
				break
			}
		}
	}
	return out, true
}

// Entries binds a mapping to a map type t with string keys.
func (n genericNode) Entries(t reflect.Type) (map[any]backend.KeyNode, bool) {
	m, ok := genericMap(n.value)
	if !ok || t.Key().Kind() != reflect.String {
		return nil, false
	}
	out := make(map[any]backend.KeyNode, len(m))
	for k, v := range m {
		out[reflect.ValueOf(k).Convert(t.Key()).Interface()] = genericNode{value: v, tag: n.tag}
	}
	return out, true
}

// Elems binds a sequence to slice or array type t.
func (n genericNode) Elems(reflect.Type) ([]backend.KeyNode, bool) {
	v := reflect.ValueOf(n.value)
	if v.Kind() != reflect.Slice {
		return nil, false
	}
	out := make([]backend.KeyNode, v.Len())
	for i := range v.Len() {
		out[i] = genericNode{value: v.Index(i).Interface(), tag: n.tag}
	}
	return out, true
}

// genericMap returns a generically decoded mapping with string keys.
func genericMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, v := range m {
			if s, ok := k.(string); ok {
				out[s] = v
			}
		}
		return out, true
	default:
		return nil, false
	}
}
