// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader

import (
	"fmt"
	"reflect"

	"github.com/altessa-s/go-atlas/core/types/nilcheck"
)

// loadDefaultValues parses and sets default values for configuration fields.
// It processes both struct tags and Defaulter interface implementations. A
// field whose key is present in a configuration file keeps the file value even
// when it is false, 0 or "".
func (cf *Config) loadDefaultValues() (err error) {
	if !cf.options.skipDefaults {
		if df, ok := cf.conf.(Defaulter); ok {
			df.Default()
		}
	}

	cf.fields = structFields(cf.conf)
	for f := range cf.fields.All() {
		if !cf.options.skipDefaults && !f.isStructPtr() && !cf.setInFile(f) {
			if err = f.setDefaultValue(defaultValueTagName, cf.options.strict); err != nil {
				return err
			}
		}

		if !cf.options.skipDefaults && f.isStructPtr() && nilcheck.IsNotNilValue(f.value) {
			if df, ok := f.value.Interface().(Defaulter); ok {
				df.Default()
			}
		}
	}

	return
}

// setInFile reports whether a configuration file set the key of f. Inline
// ancestors contribute no key of their own.
func (cf *Config) setInFile(f *field) bool {
	tagName := cf.backend.StructTagName()

	var keys []string
	for cur := f; cur != nil; cur = cur.parent {
		if isInline(cur.field, tagName) {
			continue
		}
		key := fileKey(cur.field, tagName)
		if key == "" {
			return false
		}
		keys = append(keys, key)
	}

	p := cf.present
	for i := len(keys) - 1; i >= 0; i-- {
		var ok bool
		if p, ok = p.child(keys[i]); !ok {
			return false
		}
	}
	return true
}

// applyDefaultsToMaps applies default values to struct elements of maps and
// slices, which the field list does not reach.
func (cf *Config) applyDefaultsToMaps(value any) error {
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}

	return cf.applyDefaultsToValue(v, cf.present, false)
}

// applyDefaultsToValue recursively applies defaults to a reflect.Value. p is
// the file presence node for v; inElement reports whether v lies inside a map
// or slice element, where the field list does not apply defaults itself.
func (cf *Config) applyDefaultsToValue(v reflect.Value, p presence, inElement bool) error {
	if !v.IsValid() || !v.CanInterface() {
		return nil
	}

	switch v.Kind() {
	case reflect.Struct:
		if inElement {
			return cf.applyDefaultsToStruct(v, p)
		}
		return cf.applyDefaultsToStructFields(v, p)
	case reflect.Map:
		return cf.applyDefaultsToMap(v, p)
	case reflect.Slice, reflect.Array:
		return cf.applyDefaultsToSlice(v, p)
	case reflect.Pointer:
		if v.IsNil() {
			return nil
		}
		return cf.applyDefaultsToValue(v.Elem(), p, inElement)
	default:
		return nil
	}
}

// fieldPresence returns the presence node of struct field sf under p and
// whether the field's key was set; inline fields share the parent's node.
func (cf *Config) fieldPresence(sf reflect.StructField, p presence) (presence, bool) {
	tagName := cf.backend.StructTagName()
	if isInline(sf, tagName) {
		return p, false
	}
	key := fileKey(sf, tagName)
	if key == "" {
		return presence{}, false
	}
	return p.child(key)
}

// applyDefaultsToStructFields recurses into the fields of a struct whose own
// defaults are applied by the field list.
func (cf *Config) applyDefaultsToStructFields(v reflect.Value, p presence) error {
	for i := range v.NumField() {
		field := v.Field(i)
		if !field.CanSet() {
			continue
		}
		fp, _ := cf.fieldPresence(v.Type().Field(i), p)
		if err := cf.applyDefaultsToValue(field, fp, false); err != nil {
			return err
		}
	}
	return nil
}

// applyDefaultsToMap applies defaults to struct values inside a map.
func (cf *Config) applyDefaultsToMap(v reflect.Value, p presence) error {
	if v.IsNil() {
		return nil
	}

	valueType := v.Type().Elem()
	if !cf.isStructValueType(valueType) {
		return nil
	}

	for _, mapKey := range cf.collectMapKeys(v) {
		ep, _ := p.child(fmt.Sprint(mapKey.Interface()))
		if err := cf.processMapEntry(v, valueType, mapKey, v.MapIndex(mapKey), ep); err != nil {
			return err
		}
	}
	return nil
}

// isStructValueType checks if a type is a struct or pointer to struct.
func (cf *Config) isStructValueType(valueType reflect.Type) bool {
	return valueType.Kind() == reflect.Struct ||
		(valueType.Kind() == reflect.Pointer && valueType.Elem().Kind() == reflect.Struct)
}

// collectMapKeys collects all keys from a map.
func (cf *Config) collectMapKeys(v reflect.Value) []reflect.Value {
	keys := make([]reflect.Value, 0, v.Len())
	iter := v.MapRange()
	for iter.Next() {
		keys = append(keys, iter.Key())
	}
	return keys
}

// processMapEntry processes a single map entry.
func (cf *Config) processMapEntry(v reflect.Value, valueType reflect.Type, mapKey, mapValue reflect.Value, p presence) error {
	switch mapValue.Kind() {
	case reflect.Pointer:
		if mapValue.IsNil() || mapValue.Elem().Kind() != reflect.Struct {
			return nil
		}
		return cf.applyDefaultsToStruct(mapValue.Elem(), p)
	case reflect.Struct:
		// Map values are not addressable: default a copy and store it back.
		newValue := reflect.New(valueType).Elem()
		newValue.Set(mapValue)
		if err := cf.applyDefaultsToStruct(newValue, p); err != nil {
			return err
		}
		v.SetMapIndex(mapKey, newValue)
		return nil
	default:
		return nil
	}
}

// applyDefaultsToSlice applies defaults to all elements in a slice or array.
func (cf *Config) applyDefaultsToSlice(v reflect.Value, p presence) error {
	for i := range v.Len() {
		if err := cf.applyDefaultsToValue(v.Index(i), p.index(i), true); err != nil {
			return err
		}
	}
	return nil
}

// applyDefaultsToStruct applies default tags to the fields of a map or slice
// element and its nested structs, skipping fields the file set explicitly.
func (cf *Config) applyDefaultsToStruct(structValue reflect.Value, p presence) error {
	structType := structValue.Type()

	for i := range structType.NumField() {
		field := structType.Field(i)
		fieldValue := structValue.Field(i)
		if !fieldValue.CanSet() {
			continue
		}

		fp, explicit := cf.fieldPresence(field, p)

		defaultTag := field.Tag.Get(defaultValueTagName)
		if defaultTag != "" && !explicit && fieldValue.IsZero() {
			if cf.options.strict {
				var subErr error
				if defaultTag, subErr = substituteEnvVariablesStrict(defaultTag); subErr != nil {
					return subErr
				}
			} else {
				defaultTag = substituteEnvVariables(defaultTag)
			}

			// isDefaultValue=false bypasses the zero check already done above.
			if err := set(fieldValue, defaultTag, false, true, cf.options.strict); err != nil {
				return err
			}
		}

		if err := cf.applyDefaultsToValue(fieldValue, fp, true); err != nil {
			return err
		}
	}

	return nil
}
