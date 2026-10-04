// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader

import (
	"reflect"
	"slices"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"
)

// loadDefaultValues parses and sets default values for configuration fields.
// It processes both struct tags and Defaulter interface implementations. A
// field a configuration file or the environment set keeps its value even when
// it is false, 0 or "".
func (cf *Config) loadDefaultValues() (err error) {
	if !cf.options.skipDefaults {
		cf.callDefaulter(reflect.ValueOf(cf.conf).Elem(), cf.present)
	}

	cf.fields = structFields(cf.conf)
	for f := range cf.fields.All() {
		p := cf.present.at(f.index)
		if !cf.options.skipDefaults && !f.isStructPtr() && !p.isSet() {
			if err = f.setDefaultValue(defaultValueTagName, cf.options.strict); err != nil {
				return err
			}
		}

		if !cf.options.skipDefaults && f.isStructPtr() && nilcheck.IsNotNilValue(f.value) {
			cf.callDefaulter(f.value.Elem(), p)
		}
	}

	return
}

// callDefaulter runs Default on struct v when it implements Defaulter, keeping
// the values the files and the environment set: Default may fill or derive
// the other fields but never overrides an explicit value. p is the presence of
// v.
func (cf *Config) callDefaulter(v reflect.Value, p *presence) {
	if !v.CanAddr() {
		return
	}
	d, ok := v.Addr().Interface().(Defaulter)
	if !ok {
		return
	}

	explicit := collectExplicit(v, p, nil, nil)
	d.Default()
	for _, e := range explicit {
		e.restore(v)
	}
}

// explicitValue is a copy of an explicitly set value of a struct, located by
// its field index path.
type explicitValue struct {
	value reflect.Value
	path  []int
}

// collectExplicit copies the explicitly set values of struct v. It descends
// into nested structs whose fields are known, so Default can still fill their
// other fields, and copies any other value — scalars, maps and slices, an
// explicit null pointer — whole.
func collectExplicit(v reflect.Value, p *presence, path []int, out []explicitValue) []explicitValue {
	if p == nil {
		return out
	}
	for i, c := range p.fields {
		if c == nil {
			continue
		}
		fv := v.Field(i)
		if !fv.CanSet() {
			continue
		}
		fieldPath := append(slices.Clip(path), i)

		if c.fields != nil && !c.null {
			sv := fv
			for sv.Kind() == reflect.Pointer && !sv.IsNil() {
				sv = sv.Elem()
			}
			if sv.Kind() == reflect.Struct {
				out = collectExplicit(sv, c, fieldPath, out)
				continue
			}
		}

		if c.set || len(c.entries) > 0 || len(c.elems) > 0 {
			cp := reflect.New(fv.Type()).Elem()
			cp.Set(fv)
			out = append(out, explicitValue{value: cp, path: fieldPath})
		}
	}
	return out
}

// restore writes the copied value back into root, allocating pointer structs
// along the path that Default reset.
func (e explicitValue) restore(root reflect.Value) {
	v := root
	for _, i := range e.path {
		for v.Kind() == reflect.Pointer {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	v.Set(e.value)
}

// applyDefaultsToMaps applies default values to struct elements of maps and
// slices, which the field list does not reach.
func (cf *Config) applyDefaultsToMaps(value any) error {
	if cf.options.skipDefaults {
		return nil
	}

	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}

	return cf.applyDefaultsToStructFields(v, cf.present)
}

// applyDefaultsToStructFields walks the fields of a struct whose own defaults
// the field list applies, looking for collections whose elements it does not
// reach.
func (cf *Config) applyDefaultsToStructFields(v reflect.Value, p *presence) error {
	for i := range v.NumField() {
		field := v.Field(i)
		if !field.CanSet() {
			continue
		}
		fp := p.field(i)

		for field.Kind() == reflect.Pointer && !field.IsNil() {
			field = field.Elem()
		}
		var err error
		switch field.Kind() {
		case reflect.Struct:
			err = cf.applyDefaultsToStructFields(field, fp)
		case reflect.Map, reflect.Slice, reflect.Array:
			err = cf.applyDefaultsToCollection(field, fp)
		default:
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// applyDefaultsToCollection applies defaults to the struct elements of a map,
// slice or array, recursing into nested collections.
func (cf *Config) applyDefaultsToCollection(v reflect.Value, p *presence) error {
	switch v.Kind() {
	case reflect.Map:
		return cf.applyDefaultsToMap(v, p)
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			if err := cf.applyDefaultsToElement(v.Index(i), p.elem(i)); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

// applyDefaultsToElement applies defaults to an addressable map or slice
// element.
func (cf *Config) applyDefaultsToElement(v reflect.Value, p *presence) error {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Struct:
		return cf.applyDefaultsToStruct(v, p, true)
	case reflect.Map, reflect.Slice, reflect.Array:
		return cf.applyDefaultsToCollection(v, p)
	default:
		return nil
	}
}

// applyDefaultsToMap applies defaults to the values of a map. Values are not
// addressable, so each one is defaulted as a copy and stored back.
func (cf *Config) applyDefaultsToMap(v reflect.Value, p *presence) error {
	if v.IsNil() {
		return nil
	}
	valueType := v.Type().Elem()
	if !hasStructElements(valueType) {
		return nil
	}

	iter := v.MapRange()
	for iter.Next() {
		key := iter.Key()
		value := reflect.New(valueType).Elem()
		value.Set(iter.Value())
		if err := cf.applyDefaultsToElement(value, p.entry(key)); err != nil {
			return err
		}
		v.SetMapIndex(key, value)
	}
	return nil
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

// applyDefaultsToStruct applies defaults to a struct inside a map or slice
// element, which the field list does not reach. Like the field list does for
// the configuration and its nested pointer structs, it first runs Default when
// withDefaulter is set — for the element itself and pointer structs, not for
// struct values — then applies default tags to the zero fields nothing set
// explicitly, and allocates nil pointer structs unless they are tagged
// default:"-" or a file set them to null.
func (cf *Config) applyDefaultsToStruct(structValue reflect.Value, p *presence, withDefaulter bool) error {
	if withDefaulter {
		cf.callDefaulter(structValue, p)
	}

	structType := structValue.Type()
	for i := range structType.NumField() {
		field := structType.Field(i)
		fieldValue := structValue.Field(i)
		if !fieldValue.CanSet() {
			continue
		}

		fp := p.field(i)
		defaultTag := field.Tag.Get(defaultValueTagName)

		if isStructPointer(field.Type) {
			// On a pointer to a struct "-" is the do-not-allocate sentinel,
			// not a value.
			if fieldValue.IsNil() && defaultTag != "-" && !fp.isNull() && allocatedByFieldList(field.Type.Elem(), nil) {
				fieldValue.Set(reflect.New(field.Type.Elem()))
			}
			if !fieldValue.IsNil() {
				if err := cf.applyDefaultsToStruct(fieldValue.Elem(), fp, true); err != nil {
					return err
				}
			}
			continue
		}

		if defaultTag != "" && !fp.isSet() && fieldValue.IsZero() {
			if err := cf.setElementDefault(fieldValue, defaultTag); err != nil {
				return err
			}
		}

		var err error
		switch fieldValue.Kind() {
		case reflect.Struct:
			err = cf.applyDefaultsToStruct(fieldValue, fp, false)
		case reflect.Map, reflect.Slice, reflect.Array:
			err = cf.applyDefaultsToCollection(fieldValue, fp)
		default:
		}
		if err != nil {
			return err
		}
	}

	return nil
}

// setElementDefault substitutes environment variables in a default tag and
// assigns it to an element field.
func (cf *Config) setElementDefault(fieldValue reflect.Value, defaultTag string) error {
	if cf.options.strict {
		var err error
		if defaultTag, err = substituteEnvVariablesStrict(defaultTag); err != nil {
			return err
		}
	} else {
		defaultTag = substituteEnvVariables(defaultTag)
	}

	// isDefaultValue=false bypasses the zero check the caller already did.
	return set(fieldValue, defaultTag, false, true, cf.options.strict)
}

// allocatedByFieldList reports whether the field list allocates a nil pointer
// to struct type t when applying defaults: it does when t, or a struct t
// embeds, has an exported field other than a pointer to a struct, since
// defaulting that field initializes its parent. A struct without such fields,
// like time.Time, stays nil.
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
