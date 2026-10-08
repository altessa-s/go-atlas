// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"cmp"
	"fmt"
	"reflect"
	"slices"
	"sync"

	"github.com/altessa-s/go-atlas/domain/behavior"
)

// docField is one top-level key of a struct's BSON document.
type docField struct {
	// name is the document key.
	name string
	// index is the Go FieldByIndex path from the struct to the field; it is
	// longer than one element only for a field imported from an inline struct.
	index []int
	// goName is the Go name of the field at the end of index.
	goName string
	// tag is the struct tag of the field at the end of index.
	tag reflect.StructTag
	// omitEmpty is the field's own omitempty option.
	omitEmpty bool
	// omitOnUpdate is set when the field or an inline struct it was imported
	// through is tagged omitonupdate.
	omitOnUpdate bool
	// minSize is the field's own minsize option.
	minSize bool
}

// docLayout is the document shape of a struct type as the v2 BSON struct
// codec lays it out (bson/struct_codec.go describeStruct).
type docLayout struct {
	// fields are the dominant keys, ordered by Go index path.
	fields []docField
	// inlineMap is the Go index of the struct's own inline map field, or -1.
	// An inline map inside an inline struct is not propagated: the codec
	// writes none of its entries.
	inlineMap int
	// names holds every key of fields; an inline map entry may not reuse one.
	names map[string]struct{}
}

// layoutKey identifies a layout: the same type parses differently under
// another tag key.
type layoutKey struct {
	t   reflect.Type
	tag string
}

// layoutEntry memoizes a layout or the error describing the type produced.
type layoutEntry struct {
	layout *docLayout
	err    error
}

// layoutCache memoizes layoutOf. Struct types are finite, so an unbounded
// sync.Map suffices.
var layoutCache sync.Map // map[layoutKey]layoutEntry

// layoutOf returns the document layout of struct type t under tag key tag,
// rejecting the declarations the codec rejects: an inline field that is not a
// struct, a pointer to a struct or a map with exactly string keys; more than
// one inline map; two keys of the same name at the same, shallowest depth.
func layoutOf(t reflect.Type, tag string) (*docLayout, error) {
	key := layoutKey{t: t, tag: tag}
	if v, ok := layoutCache.Load(key); ok {
		e, _ := v.(layoutEntry)
		return e.layout, e.err
	}
	l, err := describe(t, tag, nil)
	layoutCache.Store(key, layoutEntry{layout: l, err: err})
	return l, err
}

// describe mirrors the codec's describeStruct for one struct type. visiting
// holds the struct types on the current inline chain; a type inlined into
// itself is rejected instead of recursing forever.
func describe(t reflect.Type, tag string, visiting []reflect.Type) (*docLayout, error) {
	if slices.Contains(visiting, t) {
		return nil, fmt.Errorf("behavior/mongo: struct %s is inlined into itself", t)
	}
	visiting = append(visiting, t)
	l := &docLayout{inlineMap: -1}
	var fields []docField
	for i := range t.NumField() {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		raw := (&options{bsonTagName: tag}).fieldTag(sf.Tag)
		name, omitEmpty, omitOnUpdate := parseBSONTag(raw, sf.Name)
		if name == "" {
			continue
		}
		if !hasInline(raw) {
			fields = append(fields, docField{
				name: name, index: []int{i}, goName: sf.Name, tag: sf.Tag,
				omitEmpty: omitEmpty, omitOnUpdate: omitOnUpdate, minSize: bsonTag(raw, sf.Name).minSize,
			})
			continue
		}

		st := sf.Type
		switch st.Kind() {
		case reflect.Map:
			if l.inlineMap >= 0 {
				return nil, fmt.Errorf("behavior/mongo: struct %s: multiple inline maps", t)
			}
			if st.Key() != reflect.TypeFor[string]() {
				return nil, fmt.Errorf("behavior/mongo: struct %s: inline map must have string keys", t)
			}
			l.inlineMap = i
			continue
		case reflect.Pointer:
			st = st.Elem()
			if st.Kind() != reflect.Struct {
				return nil, fmt.Errorf("behavior/mongo: struct %s: inline fields must be a struct, a struct pointer, or a map", t)
			}
		case reflect.Struct:
		default:
			return nil, fmt.Errorf("behavior/mongo: struct %s: inline fields must be a struct, a struct pointer, or a map", t)
		}
		inner, err := describe(st, tag, visiting)
		if err != nil {
			return nil, err
		}
		for _, fd := range inner.fields {
			fd.index = append([]int{i}, fd.index...)
			fd.omitOnUpdate = fd.omitOnUpdate || omitOnUpdate
			fields = append(fields, fd)
		}
	}

	// Sort by name, then depth, then index; the first field of each name
	// dominates, unless the next one sits at the same depth.
	slices.SortStableFunc(fields, func(a, b docField) int {
		return cmp.Or(
			cmp.Compare(a.name, b.name),
			cmp.Compare(len(a.index), len(b.index)),
			slices.Compare(a.index, b.index),
		)
	})
	l.names = make(map[string]struct{}, len(fields))
	for i := 0; i < len(fields); {
		j := i + 1
		for j < len(fields) && fields[j].name == fields[i].name {
			j++
		}
		if j-i > 1 && len(fields[i].index) == len(fields[i+1].index) {
			return nil, fmt.Errorf("behavior/mongo: struct %s has duplicated key %s", t, fields[i].name)
		}
		l.fields = append(l.fields, fields[i])
		l.names[fields[i].name] = struct{}{}
		i = j
	}
	slices.SortFunc(l.fields, func(a, b docField) int { return slices.Compare(a.index, b.index) })
	return l, nil
}

// objectLayout returns the layout of the struct obj was resolved from.
func (o *options) objectLayout(obj behavior.Object) (*docLayout, error) {
	if !obj.Value.IsValid() || obj.Value.Kind() != reflect.Struct {
		return nil, fmt.Errorf("behavior/mongo: object has no struct value")
	}
	return layoutOf(obj.Value.Type(), o.bsonTagName)
}

// located is the outcome of [locate].
type located struct {
	// field is the resolved field at the index path, or the stripped ancestor
	// that governs it when exact is false.
	field *behavior.Field
	// exact reports that field sits at the index path itself.
	exact bool
	// owner and rest are where the search stopped without a field: the object
	// reached and the index path remaining within it. A promoted embedded
	// struct is rebuilt from there (see [promotedField]).
	owner behavior.Object
	rest  []int
}

// locate finds the resolved field at Go index path index within obj,
// following Nested objects through inline structs. With no field at the
// path it reports the deepest object reached, so a struct whose exported
// fields the engine promoted can be rebuilt from its owner.
func locate(obj behavior.Object, index []int) located {
	for i := range obj.Fields {
		fi := &obj.Fields[i]
		switch {
		case slices.Equal(fi.Index, index):
			return located{field: fi, exact: true}
		case len(fi.Index) < len(index) && slices.Equal(fi.Index, index[:len(fi.Index)]):
			if fi.Strip {
				return located{field: fi}
			}
			if fi.Nested != nil {
				return locate(*fi.Nested, index[len(fi.Index):])
			}
			// A nil pointer the walk did not descend: nothing below it.
			return located{}
		}
	}
	return located{owner: obj, rest: index}
}

// resolveField returns the field a layout entry addresses within obj: the
// resolved field itself, or one rebuilt for an embedded struct whose exported
// fields the engine promoted. ok is false when nothing is stored at the path
// (behind a nil pointer). A stripped ancestor is returned as is with exact
// false; the caller decides what its Strip mark means.
func resolveField(obj behavior.Object, df docField) (f *behavior.Field, exact, ok bool) {
	loc := locate(obj, df.index)
	switch {
	case loc.field != nil:
		return loc.field, loc.exact, true
	case loc.owner.Value.IsValid():
		pf, reached := promotedField(loc.owner, loc.rest, df)
		if !reached {
			return nil, false, false
		}
		return &pf, true, true
	default:
		return nil, false, false
	}
}

// hasPromoted reports whether owner holds a field promoted from under rest.
func hasPromoted(owner behavior.Object, rest []int) bool {
	return slices.ContainsFunc(owner.Fields, func(pf behavior.Field) bool {
		return len(pf.Index) > len(rest) && slices.Equal(pf.Index[:len(rest)], rest)
	})
}

// promotedField rebuilds the field at index path rest within owner for an
// embedded struct the engine flattened: its value comes from owner's struct
// value and its Nested object gathers the promoted fields, re-rooted at the
// embedded struct. Behind a nil embedded pointer the value stays nil — the
// document semantics of a nil pointer — while Nested still gathers whatever
// promoted fields schema-walk synthesized, so type-driven translators see
// them. ok is false when an ancestor of rest is a nil pointer and nothing was
// synthesized behind it.
func promotedField(owner behavior.Object, rest []int, df docField) (behavior.Field, bool) {
	fv, err := owner.Value.FieldByIndexErr(rest)
	if err != nil {
		// A nil pointer above rest: no value is stored. Only schema-walk
		// synthesizes promoted fields behind it; for those, continue with a
		// zero of the declared type so path translators still see them.
		if !hasPromoted(owner, rest) {
			return behavior.Field{}, false
		}
		fv = reflect.Zero(owner.Value.Type().FieldByIndex(rest).Type)
	}
	f := behavior.Field{Index: rest, Name: df.goName, Tag: df.tag, Value: fv}

	st := fv.Type()
	if st.Kind() == reflect.Pointer {
		st = st.Elem()
	}
	if st.Kind() != reflect.Struct {
		return f, true
	}
	var fields []behavior.Field
	for _, pf := range owner.Fields {
		if len(pf.Index) > len(rest) && slices.Equal(pf.Index[:len(rest)], rest) {
			pf.Index = pf.Index[len(rest):]
			fields = append(fields, pf)
		}
	}
	sv := reflect.Indirect(fv)
	if !sv.IsValid() {
		if len(fields) == 0 {
			return f, true
		}
		sv = reflect.Zero(st)
	}
	f.Nested = &behavior.Object{Fields: fields, Value: sv}
	return f, true
}
