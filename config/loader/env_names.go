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

// fieldEnvNames is the environment naming of one struct field: it depends
// on the field and the struct tag key only, so it is computed once per struct
// type (see structEnvNames).
type fieldEnvNames struct {
	// levels are the names the field answers to as one level of a variable
	// name: its struct tag name and its Go field name, in
	// SCREAMING_SNAKE_CASE, without duplicates.
	levels []string
	// envTag is the field's `env` tag.
	envTag string
	// valueEnvTag reports whether the field has an `env` tag and holds a
	// value rather than a struct section (see hasValueEnvTag).
	valueEnvTag bool
	// anonymous reports whether the field is embedded.
	anonymous bool
}

// matches reports whether the field answers to name as one level of an
// environment variable name. fixed reports whether the level is reached
// through struct fields only, where a value field with an `env` tag is named
// by the tag and so never matches by its path name.
func (n *fieldEnvNames) matches(name string, fixed bool) bool {
	if fixed && n.valueEnvTag {
		return false
	}
	return slices.Contains(n.levels, name)
}

// structEnvNames returns the environment naming of the fields of the struct
// type t, by field index. It is cached in the Config across loads, as it
// depends on the type and the struct tag key only; callers must not modify
// the result.
func (cf *Config) structEnvNames(t reflect.Type) []fieldEnvNames {
	if names, ok := cf.envNames[t]; ok {
		return names
	}
	names := make([]fieldEnvNames, t.NumField())
	for i := range names {
		names[i] = cf.computeFieldEnvNames(t.Field(i))
	}
	if cf.envNames == nil {
		cf.envNames = make(map[reflect.Type][]fieldEnvNames)
	}
	cf.envNames[t] = names
	return names
}

// computeFieldEnvNames computes the environment naming of the struct field.
func (cf *Config) computeFieldEnvNames(sf reflect.StructField) fieldEnvNames {
	return fieldEnvNames{
		levels:      cf.computeLevelNames(sf),
		envTag:      sf.Tag.Get(envTagName),
		valueEnvTag: hasValueEnvTag(sf),
		anonymous:   sf.Anonymous,
	}
}

// fieldNames returns the environment naming of a field: from the cache of
// the struct type declaring it when known, else computed.
func (cf *Config) fieldNames(fld *field) *fieldEnvNames {
	if fld.owner != nil && len(fld.field.Index) > 0 {
		return &cf.structEnvNames(fld.owner)[fld.field.Index[len(fld.field.Index)-1]]
	}
	names := cf.computeFieldEnvNames(fld.field)
	return &names
}

// computeLevelNames computes the level names of the struct field (see
// fieldEnvNames).
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

// envFieldNames returns the environment variable names of a field from the
// field list, without the configured prefix (see the rule above). It returns
// nil for pointer struct sections, which are reached through their fields.
func (cf *Config) envFieldNames(fld *field) []string {
	if fld.isStructPtr() {
		return nil
	}
	if own := cf.fieldNames(fld); own.valueEnvTag {
		if name := cf.envTagVarName(own.envTag); name != "" {
			return []string{name}
		}
		return nil // env:"-"
	}

	names := []string{""}
	for f := fld; f != nil; f = f.parent {
		levels := cf.fieldNames(f).levels
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

// envTagVarName returns the variable name the `env` tag envTag gives,
// without the configured prefix, or "" when the field has no tag or the tag
// is "-", which keeps the environment out of the field. With a prefix
// configured, a tag starting with the "APP_" placeholder or with the prefix
// has that part removed, so it maps to the prefix followed by the rest of the
// tag; a tag that is only the placeholder or the prefix names itself.
func (cf *Config) envTagVarName(envTag string) string {
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

// taggedField is a value field of the field list with an `env` tag and the
// variable name the tag gives (see envTagVarName).
type taggedField struct {
	fld  *field
	name string
}

// collectEnvTagged returns the value fields of the field list whose `env`
// tag names a variable, with that name.
func (cf *Config) collectEnvTagged() []taggedField {
	var tagged []taggedField
	for _, fld := range cf.fields {
		if names := cf.fieldNames(fld); names.valueEnvTag {
			if name := cf.envTagVarName(names.envTag); name != "" {
				tagged = append(tagged, taggedField{fld: fld, name: name})
			}
		}
	}
	return tagged
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
	for _, tf := range cf.envTagged {
		fld, name := tf.fld, tf.name
		rest, ok := strings.CutPrefix(key, name)
		if !ok || len(name) <= longest {
			continue
		}
		if rest == "" {
			found, entries, longest = fld, nil, len(name)
			continue
		}
		if !isCollectionType(fld.field.Type) {
			continue
		}
		if rest, ok = strings.CutPrefix(rest, delimiter); !ok {
			continue
		}
		path := strings.Split(rest, delimiter)
		t, ok := cf.entryPathType(fld.field.Type, path)
		if !ok || slice && derefType(t).Kind() != reflect.Slice {
			continue
		}
		found, entries, longest = fld, path, len(name)
	}
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
			if !hasEnvMapKeys(t) {
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

// settablePath reports whether path, from type t, leads to a value the
// nested-section pass of the environment loader sets: through struct level
// names, then map keys, where every map below the first key holds structs or
// maps, ending on a field or a map entry. It does not go through slices, which
// the indexed pass sets. fixed is passed to the struct level lookup until the
// first map key. When a struct level answers to no field, missing names it.
// Checking the path on the type first keeps a variable that sets nothing from
// allocating the nil sections and maps along it.
func (cf *Config) settablePath(t reflect.Type, path []string, fixed bool) (missing string, ok bool) {
	for i, part := range path {
		t = derefType(t)
		switch t.Kind() {
		case reflect.Map:
			if !hasEnvMapKeys(t) {
				return "", false
			}
			t, fixed = t.Elem(), false
			if i < len(path)-1 && !isNestedEntryType(t) {
				return "", false
			}
		case reflect.Struct:
			sf, found := cf.findStructFieldRecursive(t, part, fixed)
			if !found {
				return part, false
			}
			if !sf.IsExported() {
				return "", false
			}
			t = sf.Type
		default:
			return "", false
		}
	}
	return "", true
}

// indexedPathType returns the type that parts lead to from type t, as the
// indexed pass of the environment loader walks a variable name: a number is a
// slice index, any other level a map key or a struct level name, possibly with
// an index suffix (NAME[0]), and no level is below a value. With numericIndex
// set, as findFieldByPath walks, a number is never a map key. fixed is passed
// to the struct level lookup until the first index or map key. A negative
// index resolves, so the setters report it. When a struct level answers to no
// field, missing names it. Checking a name on the type first keeps one that
// sets nothing from allocating the nil sections, maps and elements along it.
func (cf *Config) indexedPathType(t reflect.Type, parts []string, fixed, numericIndex bool) (_ reflect.Type, missing string, ok bool) {
	for _, part := range parts {
		t = derefType(t)
		if _, err := strconv.Atoi(part); err == nil && (numericIndex || t.Kind() != reflect.Map) {
			if t.Kind() != reflect.Slice {
				return nil, "", false
			}
			t, fixed = t.Elem(), false
			continue
		}
		switch t.Kind() {
		case reflect.Map:
			if !hasEnvMapKeys(t) {
				return nil, "", false
			}
			t, fixed = t.Elem(), false
		case reflect.Struct:
			sf, found := cf.findStructFieldRecursive(t, part, fixed)
			if found {
				t = sf.Type
				continue
			}
			base, index, hasIndex := splitFieldNameAndIndex(part)
			if !hasIndex || numericIndex {
				return nil, part, false
			}
			if sf, found = cf.findStructFieldRecursive(t, base, fixed); !found {
				return nil, part, false
			}
			if _, err := strconv.Atoi(index); err != nil || derefType(sf.Type).Kind() != reflect.Slice {
				return nil, "", false
			}
			t, fixed = derefType(sf.Type).Elem(), false
		default:
			return nil, "", false
		}
	}
	return t, "", true
}

// indexedKeyResolves reports whether the indexed variable name key, split
// into parts, addresses something the indexed pass sets: through the `env`
// tag that claims it, if any, else from the configuration root (see
// indexedPathType). In strict mode a name with a struct level no field answers
// to is kept, so the setters report it.
func (cf *Config) indexedKeyResolves(key string, parts []string) bool {
	t, fixed := reflect.TypeOf(cf.conf), true
	if tagged, entryPath := cf.findTaggedCollection(key, false); tagged != nil {
		if entryPath == nil {
			return false // The plain-field pass set it.
		}
		t, fixed, parts = tagged.field.Type, false, entryPath
	}
	_, missing, ok := cf.indexedPathType(t, parts, fixed, false)
	return ok || cf.options.strict && missing != ""
}

// hasEnvMapKeys reports whether the keys of the map type t can be named by an
// environment variable level: their kind is string.
func hasEnvMapKeys(t reflect.Type) bool {
	return t.Key().Kind() == reflect.String
}

// envMapKey returns the key of the map type t that the variable name level
// part names, converted to the key type, or false when the keys cannot be
// named (see hasEnvMapKeys).
func envMapKey(t reflect.Type, part string) (reflect.Value, bool) {
	if !hasEnvMapKeys(t) {
		return reflect.Value{}, false
	}
	return reflect.ValueOf(part).Convert(t.Key()), true
}

// isNestedEntryType reports whether a map value of type t is navigated into
// by the nested-section pass: a struct, a pointer to one, or a map.
func isNestedEntryType(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Struct, reflect.Map:
		return true
	case reflect.Pointer:
		return t.Elem().Kind() == reflect.Struct
	default:
		return false
	}
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
			sf, names, found := cf.findStructFieldIn(t, part, fixed, nil)
			if !found {
				return "", 0, false
			}
			if len(names.levels) > 1 && part == names.levels[1] {
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
