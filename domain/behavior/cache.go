// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/altessa-s/go-atlas/core/types/bits"
)

// fieldInfo is the parsed, per-field structural metadata reused across calls.
// It is independent of any concrete instance; instance values are bound during
// [resolve].
type fieldInfo struct {
	// index is the FieldByIndex path to the field. It has more than one element
	// only for fields promoted from an embedded (anonymous) struct.
	index []int
	name  string
	tag   reflect.StructTag
	kinds []Kind
	// kindMask is the bit set of kinds (the closed Kind enum fits a uint8),
	// precomputed so resolve can reject a non-matching field with a single AND
	// against the strip-set mask.
	kindMask uint8
	// recurse reports whether the walk should descend into this field when none
	// of its own kinds match the strip set. False for scalars and for opaque
	// structs without exported fields (time.Time, optional.Optional).
	recurse bool
}

// cacheKey distinguishes parses of the same type under different tag names.
type cacheKey struct {
	t   reflect.Type
	tag string
}

// fieldCache memoizes parsed field metadata, keyed by (type, tag name). Tag
// names are expected to be a small set of compile-time constants (the default
// "behavior" plus any [WithTagName] override); the cache is unbounded, so do
// not feed [WithTagName] caller- or user-supplied dynamic strings.
var fieldCache sync.Map // map[cacheKey][]fieldInfo

// fieldsFor returns the parsed metadata for struct type t under tag, building
// and caching it on first use. It returns an error for an unrecognized behavior
// token; that result is not cached.
func fieldsFor(t reflect.Type, tag string) ([]fieldInfo, error) {
	key := cacheKey{t: t, tag: tag}
	if v, ok := fieldCache.Load(key); ok {
		infos, _ := v.([]fieldInfo)
		return infos, nil
	}

	var infos []fieldInfo
	if err := collectFields(t, tag, nil, &infos); err != nil {
		return nil, err
	}

	fieldCache.Store(key, infos)
	return infos, nil
}

// collectFields appends the metadata for every exported field of t to out,
// prefixing each field index with indexPrefix. Untagged embedded (anonymous)
// structs — by value or by pointer — are inlined so their fields promote to the
// parent, matching encoding/json and the BSON driver. An embedded struct that
// itself carries a behavior tag is treated as an ordinary field instead.
func collectFields(t reflect.Type, tag string, indexPrefix []int, out *[]fieldInfo) error {
	for i := range t.NumField() {
		f := t.Field(i)
		index := append(append([]int(nil), indexPrefix...), i)

		kinds, err := parseTag(f.Tag.Get(tag), f.Name)
		if err != nil {
			return err
		}

		// Inline an untagged embedded struct (value or pointer) so its fields
		// promote to the parent. Go promotes the exported fields even when the
		// embedded type itself is unexported, so this runs before the
		// exported-field check; the recursion still skips the embedded struct's
		// own unexported fields. A nil embedded pointer is handled at bind time
		// ([resolve] uses FieldByIndexErr and skips unreachable promoted fields).
		if f.Anonymous && len(kinds) == 0 {
			if et := indirectStructType(f.Type); et != nil {
				if err := collectFields(et, tag, index, out); err != nil {
					return err
				}
				continue
			}
		}

		if !f.IsExported() {
			continue
		}

		var kindMask uint8
		for _, k := range kinds {
			kindMask = bits.SetBit(kindMask, uint64(k))
		}

		*out = append(*out, fieldInfo{
			index:    index,
			name:     f.Name,
			tag:      f.Tag,
			kinds:    kinds,
			kindMask: kindMask,
			recurse:  walkableType(f.Type),
		})
	}

	return nil
}

// indirectStructType returns the struct type behind t, unwrapping a single
// pointer, or nil if t is not a struct or pointer-to-struct.
func indirectStructType(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() == reflect.Struct {
		return t
	}
	return nil
}

// parseTag splits a `behavior:"a,b"` tag into its kinds. An empty or "-"
// tag yields none. An unrecognized token is an error so typos surface loudly.
func parseTag(tag, field string) ([]Kind, error) {
	if tag == "" || tag == "-" {
		return nil, nil
	}

	var out []Kind
	for token := range strings.SplitSeq(tag, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		k, ok := ParseKind(token)
		if !ok {
			return nil, fmt.Errorf("behavior: unknown behavior %q in tag of field %s", token, field)
		}
		out = append(out, k)
	}

	return out, nil
}

// walkableType reports whether [Strip] may find behavior-tagged fields nested
// inside a field of type t. It unwraps pointers, slices, arrays, and maps to
// their element type and bottoms out at a struct with exported fields. Opaque
// structs (no exported fields, e.g. time.Time or optional.Optional), interfaces
// (the dynamic type is unknown), and scalars are leaves.
func walkableType(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return walkableType(t.Elem())
	case reflect.Struct:
		return hasExportedField(t)
	default:
		return false
	}
}

func hasExportedField(t reflect.Type) bool {
	for i := range t.NumField() {
		if t.Field(i).IsExported() {
			return true
		}
	}
	return false
}
