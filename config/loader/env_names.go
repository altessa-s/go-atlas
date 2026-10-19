// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package loader

import (
	"reflect"
	"slices"
	"strconv"
	"strings"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

// Environment variable names. Every path of the environment loader (plain
// fields, indexed slices, nested sections and map entries) follows one rule:
//
//   - With a prefix configured, only the variables carrying it are read, and
//     the prefix is stripped before the rest of the name is matched.
//   - A field holding a value (anything but a struct section) with an `env`
//     tag is named by the tag alone; its path name does not apply. Entries of
//     such a slice or map field are addressed as TAG__<index|key>, also when
//     the tag contains the delimiter (see findTaggedCollection).
//   - Any other field is named by its path: one name per level joined by the
//     section delimiter ("__" by default). A level answers to its struct tag
//     name (yaml unless WithStructTag says otherwise) and to its Go field
//     name, both in SCREAMING_SNAKE_CASE, matched exactly. Embedded structs
//     add no level. Slice indices and map keys are path levels taken
//     verbatim.
//   - Inside slice elements and map values, where a fixed name cannot address
//     one field, `env` tags are ignored and the path name applies.
//   - A value field tagged env:"-" is not read from the environment, except
//     inside slice elements and map values.
//   - A value set in several spellings is taken from the struct tag spelling
//     (see envTarget).

// levelKey identifies the level names of a struct field: they depend on its
// name and tags only.
type levelKey struct {
	name string
	tag  reflect.StructTag
}

// levelNames returns the names the struct field answers to as one level of
// an environment variable name: its struct tag name and its Go field name, in
// SCREAMING_SNAKE_CASE, without duplicates. Callers must not modify the
// result, which is cached.
func (cf *Config) levelNames(sf reflect.StructField) []string {
	key := levelKey{name: sf.Name, tag: sf.Tag}
	if names, ok := cf.levels[key]; ok {
		return names
	}
	names := cf.computeLevelNames(sf)
	if cf.levels == nil {
		cf.levels = make(map[levelKey][]string)
	}
	cf.levels[key] = names
	return names
}

// computeLevelNames computes levelNames.
func (cf *Config) computeLevelNames(sf reflect.StructField) []string {
	fieldName := corestrings.ToScreamingSnakeCase(sf.Name)
	tagName := structTagName(sf, cf.options.structTag)
	if tagName == "" {
		return []string{fieldName}
	}
	tagName = corestrings.ToScreamingSnakeCase(tagName)
	if tagName == fieldName {
		return []string{fieldName}
	}
	return []string{tagName, fieldName}
}

// levelMatches reports whether the struct field answers to name as one level
// of an environment variable name. fixed reports whether the level is reached
// through struct fields only, where a value field with an `env` tag is named
// by the tag and so never matches by its path name.
func (cf *Config) levelMatches(sf reflect.StructField, name string, fixed bool) bool {
	if fixed && hasValueEnvTag(sf) {
		return false
	}
	return slices.Contains(cf.levelNames(sf), name)
}

// envFieldNames returns the environment variable names of a field from the
// field list, without the configured prefix (see the rule above). It returns
// nil for pointer struct sections, which are reached through their fields.
func (cf *Config) envFieldNames(fld *field) []string {
	if fld.isStructPtr() {
		return nil
	}
	if hasValueEnvTag(fld.field) {
		if name := cf.envTagVarName(fld.field); name != "" {
			return []string{name}
		}
		return nil // env:"-"
	}

	names := []string{""}
	for f := fld; f != nil; f = f.parent {
		levels := cf.levelNames(f.field)
		next := make([]string, 0, len(names)*len(levels))
		for _, level := range levels {
			for _, rest := range names {
				if rest == "" {
					next = append(next, level)
					continue
				}
				next = append(next, level+cf.options.envSectionDelimiter+rest)
			}
		}
		names = next
	}
	return names
}

// envTagVarName returns the variable name the field's `env` tag gives,
// without the configured prefix, or "" when the field has no tag or the tag
// is "-", which keeps the environment out of the field. With a prefix
// configured, a tag starting with the "APP_" placeholder or with the prefix
// has that part removed, so it maps to the prefix followed by the rest of the
// tag; a tag that is only the placeholder or the prefix names itself.
func (cf *Config) envTagVarName(sf reflect.StructField) string {
	envTag := sf.Tag.Get(envTagName)
	if envTag == "" || envTag == "-" {
		return ""
	}
	if prefix := cf.options.envPrefix; prefix != "" {
		if rest, ok := strings.CutPrefix(envTag, appEnvTagPrefix); ok && rest != "" {
			return rest
		}
		if rest, ok := strings.CutPrefix(envTag, prefix); ok && rest != "" {
			return rest
		}
	}
	return envTag
}

// findTaggedCollection returns the field of the field list whose `env` tag
// claims key, among the tagged value fields loadEnvs collects, and the levels
// of key after the tag: the entry path into a slice or map field. A tag may
// contain the section delimiter.
//
// A tag naming all of key claims it with a nil entry path, for any value
// field. The tag of a slice or map field also claims key when it names the
// leading levels and the entry path addresses something in the field's type
// (see entryPathType); with slice set, it must lead to a slice, as the
// indexed variables address. When several tags claim key, the longest wins.
// A key no tag claims is resolved by its path names.
func (cf *Config) findTaggedCollection(key string, slice bool) (*field, []string) {
	delimiter := cf.options.envSectionDelimiter
	var (
		found   *field
		entries []string
		longest = -1
	)
	cf.envTagged.each(func(fld *field) bool {
		name := cf.envTagVarName(fld.field)
		rest, ok := strings.CutPrefix(key, name)
		if !ok || name == "" || len(name) <= longest {
			return true
		}
		if rest == "" {
			found, entries, longest = fld, nil, len(name)
			return true
		}
		if !isCollectionType(fld.field.Type) {
			return true
		}
		if rest, ok = strings.CutPrefix(rest, delimiter); !ok {
			return true
		}
		path := strings.Split(rest, delimiter)
		t, ok := cf.entryPathType(fld.field.Type, path)
		if !ok || slice && derefType(t).Kind() != reflect.Slice {
			return true
		}
		found, entries, longest = fld, path, len(name)
		return true
	})
	return found, entries
}

// entryPathType returns the type that path leads to from type t, through
// slice indexes, map keys and struct level names inside elements, or false
// when path addresses nothing there: an index that is not a non-negative
// number, a level no field answers to, a level below a value, or a map whose
// keys are not strings.
func (cf *Config) entryPathType(t reflect.Type, path []string) (reflect.Type, bool) {
	for _, part := range path {
		t = derefType(t)
		switch t.Kind() {
		case reflect.Slice:
			if index, err := strconv.Atoi(part); err != nil || index < 0 {
				return nil, false
			}
		case reflect.Map:
			if t.Key() != reflect.TypeFor[string]() {
				return nil, false
			}
		case reflect.Struct:
			sf, ok := cf.findStructFieldRecursive(t, part, false)
			if !ok {
				return nil, false
			}
			t = sf.Type
			continue
		default:
			return nil, false
		}
		t = t.Elem()
	}
	return t, true
}

// envTarget resolves the variable name key, without the prefix, to the value
// it sets, as a target the same for every spelling of key: the struct index
// path of the field whose env tag claims key, if any (see
// findTaggedCollection), then one canonical name per level — the Go field
// name of a struct level, the decimal number of a slice index, a map key
// verbatim. rank counts the levels key spells by Go field name where the
// struct tag name differs. ok is false when key addresses nothing.
//
// When several variables have one target, the environment loader applies
// only the one with the lowest rank, then the first in name order, so the
// struct tag spelling wins.
func (cf *Config) envTarget(key string) (target string, rank int, ok bool) {
	delimiter := cf.options.envSectionDelimiter
	var b strings.Builder
	t, fixed := reflect.TypeOf(cf.conf), true
	var path []string
	if tagged, entryPath := cf.findTaggedCollection(key, false); tagged != nil {
		for _, i := range tagged.index {
			b.WriteString("#")
			b.WriteString(strconv.Itoa(i))
		}
		t, fixed, path = tagged.field.Type, false, entryPath
	} else {
		path = strings.Split(key, delimiter)
	}
	for _, part := range path {
		t = derefType(t)
		var level string
		switch t.Kind() {
		case reflect.Slice, reflect.Array:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 {
				return "", 0, false
			}
			level = strconv.Itoa(index)
			fixed, t = false, t.Elem()
		case reflect.Map:
			level = part
			fixed, t = false, t.Elem()
		case reflect.Struct:
			sf, found := cf.findStructFieldRecursive(t, part, fixed)
			if !found {
				return "", 0, false
			}
			if names := cf.levelNames(sf); len(names) > 1 && part == names[1] {
				rank++
			}
			level = sf.Name
			t = sf.Type
		default:
			return "", 0, false
		}
		b.WriteString(delimiter)
		b.WriteString(level)
	}
	return b.String(), rank, true
}

// structTagName returns the name part of the struct tag key of sf, or ""
// when the tag is absent, empty, "-" or carries options only.
func structTagName(sf reflect.StructField, key string) string {
	name, _, _ := strings.Cut(sf.Tag.Get(key), ",")
	if name == "-" {
		return ""
	}
	return name
}

// hasValueEnvTag reports whether sf has an `env` tag and holds a value rather
// than a struct section.
func hasValueEnvTag(sf reflect.StructField) bool {
	return sf.Tag.Get(envTagName) != "" && !isStructType(sf.Type)
}

// isCollectionType reports whether t is a slice or a map, or a pointer to
// one.
func isCollectionType(t reflect.Type) bool {
	kind := derefType(t).Kind()
	return kind == reflect.Slice || kind == reflect.Map
}

// isStructType reports whether t is a struct or a pointer to one.
func isStructType(t reflect.Type) bool {
	if t == nil {
		return false
	}
	return indirectType(t).Kind() == reflect.Struct
}
