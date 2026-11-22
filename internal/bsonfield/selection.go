// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsonfield

import "reflect"

// Selection describes what a declared type reveals about its field rules.
type Selection uint8

const (
	NoFields      Selection = iota
	DynamicFields           // An interface requires inspecting its concrete value.
	SelectedFields
	OverlappingFields // A selected field contains another selected field.
)

// Inspect follows the declared types, including collection elements and map
// keys. A recursive type can occur both outside and inside a selected field.
func Inspect(t reflect.Type, matches func(reflect.Type, reflect.StructField) bool) Selection {
	type visit struct {
		typ    reflect.Type
		inside bool
	}
	seen := make(map[visit]struct{})
	var inspect func(reflect.Type, bool) Selection
	inspect = func(t reflect.Type, inside bool) Selection {
		key := visit{t, inside}
		if _, ok := seen[key]; ok {
			return NoFields
		}
		seen[key] = struct{}{}
		switch t.Kind() {
		case reflect.Interface:
			return DynamicFields
		case reflect.Array, reflect.Slice, reflect.Pointer:
			return inspect(t.Elem(), inside)
		case reflect.Map:
			return max(inspect(t.Key(), inside), inspect(t.Elem(), inside))
		case reflect.Struct:
			var result Selection
			for field := range t.Fields() {
				selected := matches(t, field)
				if selected && inside {
					return OverlappingFields
				}
				if selected {
					result = max(result, SelectedFields)
				}
				result = max(result, inspect(field.Type, inside || selected))
				if result == OverlappingFields {
					break
				}
			}
			return result
		default:
			return NoFields
		}
	}
	return inspect(t, false)
}
