// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader

import (
	"reflect"
	"slices"
)

// applyDefaults applies Default() methods and default tags to the whole
// configuration in one traversal, after the files and the environment were
// loaded. Values a file or the environment set explicitly keep their value,
// even when it is false, 0 or "".
//
// For every struct — the configuration, nested pointer structs and the struct
// elements of maps and slices — Default runs first, then default tags fill the
// zero fields nothing set explicitly. A nil pointer struct is allocated when
// it has a field to default, unless it is tagged default:"-" or a file set it
// to null.
func (cf *Config) applyDefaults() error {
	if cf.options.skipDefaults {
		return nil
	}
	return cf.defaultStruct(reflect.ValueOf(cf.conf).Elem(), cf.present, true, nil)
}

// defaultStruct applies defaults to struct v, whose presence is p. Default
// runs when withDefaulter is set: for the configuration, pointer structs and
// collection elements, not for struct values or embedded structs. path lists
// the struct types being defaulted, so a recursive pointer type is not
// allocated forever.
func (cf *Config) defaultStruct(v reflect.Value, p *presence, withDefaulter bool, path []reflect.Type) error {
	if withDefaulter {
		cf.callDefaulter(v, p)
	}

	t := v.Type()
	path = append(slices.Clip(path), t)
	for i := range t.NumField() {
		sf := t.Field(i)
		fv := v.Field(i)
		if sf.PkgPath != "" || !fv.CanSet() {
			continue
		}
		if err := cf.defaultField(fv, sf, p.field(i), path); err != nil {
			return err
		}
	}
	return nil
}

// defaultField applies defaults to the struct field fv described by sf.
func (cf *Config) defaultField(fv reflect.Value, sf reflect.StructField, p *presence, path []reflect.Type) error {
	tag := sf.Tag.Get(defaultValueTagName)

	switch {
	case isStructPointer(sf.Type):
		if fv.IsNil() {
			if !allocatesPointer(sf, tag, p, path) {
				return nil
			}
			fv.Set(reflect.New(sf.Type.Elem()))
		}
		return cf.defaultStruct(fv.Elem(), p, !sf.Anonymous, path)
	case sf.Type.Kind() == reflect.Struct:
		return cf.defaultStruct(fv, p, false, path)
	}

	if tag != "" && !p.isSet() {
		if err := cf.applyTag(fv, defaultTagValue(tag), p); err != nil {
			return err
		}
	}
	return cf.defaultElements(fv, p, path)
}

// allocatesPointer reports whether a nil pointer struct field gets allocated:
// an embedded one always is, like the field list does; another one when its
// struct has a field to default, unless it is tagged default:"-", a file set
// it to null or its type is already being defaulted.
func allocatesPointer(sf reflect.StructField, tag string, p *presence, path []reflect.Type) bool {
	elem := sf.Type.Elem()
	if slices.Contains(path, elem) {
		return false
	}
	if sf.Anonymous {
		return true
	}
	return tag != "-" && !p.isNull() && allocatedByFieldList(elem, nil)
}

// applyTag assigns a default tag value to a field nothing set explicitly.
// The tag applies to a zero value; a slice or map the environment created
// entry by entry instead gets the tag's entries the environment did not set.
func (cf *Config) applyTag(fv reflect.Value, value string, p *presence) error {
	if !needsDefault(fv) {
		return cf.fillEnvCollection(fv, value, p)
	}

	value, err := cf.substituteDefault(value)
	if err != nil {
		return err
	}
	_, err = set(fv, value, true, true, cf.options.strict)
	return err
}

// needsDefault reports whether fv, or the value it points to, is zero.
func needsDefault(fv reflect.Value) bool {
	for fv.Kind() == reflect.Pointer && !fv.IsNil() {
		fv = fv.Elem()
	}
	return fv.IsZero()
}

// fillEnvCollection adds the entries of a default tag to a slice or map that
// only the environment populated, keeping the entries it set.
func (cf *Config) fillEnvCollection(fv reflect.Value, value string, p *presence) error {
	if p == nil || p.set || fv.Kind() != reflect.Slice && fv.Kind() != reflect.Map {
		return nil
	}

	value, err := cf.substituteDefault(value)
	if err != nil {
		return err
	}
	defaults := reflect.New(fv.Type()).Elem()
	if _, err := set(defaults, value, true, true, cf.options.strict); err != nil {
		return err
	}

	if fv.Kind() == reflect.Map {
		for it := defaults.MapRange(); it.Next(); {
			if !fv.MapIndex(it.Key()).IsValid() && p.entry(it.Key()) == nil {
				fv.SetMapIndex(it.Key(), it.Value())
			}
		}
		return nil
	}

	if n := defaults.Len(); n > fv.Len() {
		grown := reflect.MakeSlice(fv.Type(), n, n)
		reflect.Copy(grown, fv)
		fv.Set(grown)
	}
	for i := range defaults.Len() {
		if p.elem(i) == nil {
			fv.Index(i).Set(defaults.Index(i))
		}
	}
	return nil
}

// substituteDefault expands ${VAR} references in a default tag value.
func (cf *Config) substituteDefault(value string) (string, error) {
	if cf.options.strict {
		return substituteEnvVariablesStrict(value)
	}
	return substituteEnvVariables(value), nil
}

// defaultElements applies defaults to the struct elements of a map, slice or
// array, recursing into nested collections. Map values are not addressable,
// so each one is defaulted as a copy and stored back.
func (cf *Config) defaultElements(v reflect.Value, p *presence, path []reflect.Type) error {
	switch v.Kind() {
	case reflect.Map:
		if v.IsNil() || !hasStructElements(v.Type().Elem()) {
			return nil
		}
		for it := v.MapRange(); it.Next(); {
			value := reflect.New(v.Type().Elem()).Elem()
			value.Set(it.Value())
			if err := cf.defaultElement(value, p.entry(it.Key()), path); err != nil {
				return err
			}
			v.SetMapIndex(it.Key(), value)
		}
	case reflect.Slice, reflect.Array:
		if !hasStructElements(v.Type().Elem()) {
			return nil
		}
		for i := range v.Len() {
			if err := cf.defaultElement(v.Index(i), p.elem(i), path); err != nil {
				return err
			}
		}
	default:
	}
	return nil
}

// defaultElement applies defaults to an addressable map or slice element.
func (cf *Config) defaultElement(v reflect.Value, p *presence, path []reflect.Type) error {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		return cf.defaultStruct(v, p, true, path)
	}
	return cf.defaultElements(v, p, path)
}

// hasStructElements reports whether values of type t are, or hold through
// pointers and nested collections, structs that may need defaults.
func hasStructElements(t reflect.Type) bool {
	for {
		switch t.Kind() {
		case reflect.Struct:
			return true
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
			t = t.Elem()
		default:
			return false
		}
	}
}

// allocatedByFieldList reports whether a nil pointer to struct type t is
// allocated when applying defaults: it is when t, or a struct t embeds, has
// an exported field other than a pointer to a struct, whose default handling
// needs the struct. A struct without such fields, like time.Time, stays nil.
func allocatedByFieldList(t reflect.Type, visiting map[reflect.Type]bool) bool {
	if visiting[t] {
		return false
	}
	if visiting == nil {
		visiting = map[reflect.Type]bool{}
	}
	visiting[t] = true

	for i := range t.NumField() {
		sf := t.Field(i)
		if sf.PkgPath != "" {
			continue
		}
		if et := derefType(sf.Type); sf.Anonymous && et.Kind() == reflect.Struct {
			if allocatedByFieldList(et, visiting) {
				return true
			}
			continue
		}
		if !isStructPointer(sf.Type) {
			return true
		}
	}
	return false
}

// isStructPointer reports whether t is a pointer to a struct.
func isStructPointer(t reflect.Type) bool {
	return t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct
}

// callDefaulter runs Default on struct v when it implements Defaulter, keeping
// the values the files and the environment set: Default may fill or derive
// any other value, but every explicit value is restored after it runs. p is
// the presence of v.
func (cf *Config) callDefaulter(v reflect.Value, p *presence) {
	if !v.CanAddr() {
		return
	}
	d, ok := v.Addr().Interface().(Defaulter)
	if !ok {
		return
	}

	explicit := takeSnapshot(v, p)
	d.Default()
	explicit.restore(v)
}

// snapshot holds independent copies of the explicitly set values under a
// value, shaped like its presence, so they can be written back after Default
// without discarding what Default set around them.
type snapshot struct {
	// value is the copy of a value restored whole: a scalar, an explicit
	// null, an explicitly empty slice or map, or a value whose structure is
	// unknown. It is invalid for a container restored child by child.
	value   reflect.Value
	fields  map[int]*snapshot
	entries []entrySnapshot
	elems   []*snapshot
	// length is the explicit length of a slice a file wrote, or -1 when only
	// some of its elements are explicit.
	length int
}

// entrySnapshot is the snapshot of a map entry.
type entrySnapshot struct {
	key  reflect.Value
	snap *snapshot
}

// takeSnapshot copies the explicitly set values under v, whose presence is p.
// It returns nil when nothing under v is explicit.
func takeSnapshot(v reflect.Value, p *presence) *snapshot {
	if p == nil || !v.IsValid() {
		return nil
	}
	if isWholeValue(v.Type(), p) {
		if !p.set && !p.null {
			return nil
		}
		return &snapshot{value: deepCopy(v)}
	}

	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}

	s := &snapshot{length: -1}
	switch v.Kind() {
	case reflect.Struct:
		s.fields = make(map[int]*snapshot, len(p.fields))
		for i, c := range p.fields {
			if !v.Type().Field(i).IsExported() {
				continue
			}
			if cs := takeSnapshot(v.Field(i), c); cs != nil {
				s.fields[i] = cs
			}
		}
	case reflect.Map:
		keyType := v.Type().Key()
		for k, c := range p.entries {
			key := reflect.ValueOf(k)
			if !key.IsValid() || !key.Type().ConvertibleTo(keyType) {
				continue
			}
			key = key.Convert(keyType)
			if cs := takeSnapshot(v.MapIndex(key), c); cs != nil {
				s.entries = append(s.entries, entrySnapshot{key: key, snap: cs})
			}
		}
	case reflect.Slice, reflect.Array:
		if p.set && v.Kind() == reflect.Slice {
			s.length = len(p.elems)
		}
		s.elems = make([]*snapshot, min(len(p.elems), v.Len()))
		for i := range s.elems {
			s.elems[i] = takeSnapshot(v.Index(i), p.elems[i])
		}
	default:
	}
	return s
}

// isWholeValue reports whether a value of type t with presence p is restored
// whole rather than child by child.
func isWholeValue(t reflect.Type, p *presence) bool {
	if p.null {
		return true
	}
	switch derefType(t).Kind() {
	case reflect.Struct:
		return p.fields == nil
	case reflect.Map:
		return p.entries == nil || p.set && len(p.entries) == 0
	case reflect.Slice:
		return p.elems == nil || p.set && len(p.elems) == 0
	case reflect.Array:
		return p.elems == nil
	default:
		return true
	}
}

// restore writes the snapshot back into the settable value v, allocating
// pointers, maps and slice elements Default removed.
func (s *snapshot) restore(v reflect.Value) {
	if s == nil {
		return
	}
	if s.value.IsValid() {
		v.Set(s.value)
		return
	}

	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		for i, c := range s.fields {
			c.restore(v.Field(i))
		}
	case reflect.Map:
		if v.IsNil() {
			v.Set(reflect.MakeMap(v.Type()))
		}
		for _, e := range s.entries {
			value := reflect.New(v.Type().Elem()).Elem()
			if current := v.MapIndex(e.key); current.IsValid() {
				value.Set(current)
			}
			e.snap.restore(value)
			v.SetMapIndex(e.key, value)
		}
	case reflect.Slice:
		n := s.length
		if n < 0 {
			n = max(v.Len(), len(s.elems))
		}
		if v.Len() != n {
			resized := reflect.MakeSlice(v.Type(), n, n)
			reflect.Copy(resized, v)
			v.Set(resized)
		}
		for i, c := range s.elems {
			c.restore(v.Index(i))
		}
	case reflect.Array:
		for i, c := range s.elems {
			c.restore(v.Index(i))
		}
	default:
	}
}

// deepCopy returns an independent copy of v: pointers, maps, slices and
// interfaces reachable through exported fields are copied, not shared.
func deepCopy(v reflect.Value) reflect.Value {
	out := reflect.New(v.Type()).Elem()
	copyInto(out, v)
	return out
}

// copyInto deep-copies src into the settable dst of the same type.
func copyInto(dst, src reflect.Value) {
	switch src.Kind() {
	case reflect.Pointer:
		if !src.IsNil() {
			p := reflect.New(src.Type().Elem())
			copyInto(p.Elem(), src.Elem())
			dst.Set(p)
		}
	case reflect.Map:
		if !src.IsNil() {
			m := reflect.MakeMapWithSize(src.Type(), src.Len())
			for it := src.MapRange(); it.Next(); {
				m.SetMapIndex(it.Key(), deepCopy(it.Value()))
			}
			dst.Set(m)
		}
	case reflect.Slice:
		if !src.IsNil() {
			s := reflect.MakeSlice(src.Type(), src.Len(), src.Len())
			for i := range src.Len() {
				copyInto(s.Index(i), src.Index(i))
			}
			dst.Set(s)
		}
	case reflect.Array:
		for i := range src.Len() {
			copyInto(dst.Index(i), src.Index(i))
		}
	case reflect.Interface:
		if !src.IsNil() {
			dst.Set(deepCopy(src.Elem()))
		}
	case reflect.Struct:
		dst.Set(src)
		for i := range src.NumField() {
			if dst.Field(i).CanSet() {
				copyInto(dst.Field(i), src.Field(i))
			}
		}
	default:
		dst.Set(src)
	}
}
