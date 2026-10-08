// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/altessa-s/go-atlas/domain/behavior"
)

// ErrStrippedInline is returned by the path-producing translators
// ([NewProjectionTranslator], [NewFieldPathsTranslator]) when stripped data
// sits in a `bson:",inline"` map — the map itself is stripped, or its values
// carry stripped fields. Its keys are dynamic and land at the parent level, so
// no dot-notation path can exclude them. Stop inlining the map, or move the
// stripped fields out of its values.
var ErrStrippedInline = errors.New("stripped data in an inline map cannot be expressed as document paths")

// pathSet accumulates the dot-notation document paths of a model.
type pathSet struct {
	// all lists every named, non-stripped field.
	all []string
	// denied lists stripped fields, plus every map whose values carry a
	// stripped field.
	denied []string
}

// collectPaths walks obj and records the document path of every field,
// recursing into nested struct and collection element schemas. With the
// engine in schema-walk mode a single representative element suffices.
//
// Field names follow the BSON struct codec's layout of each struct (see
// [layoutOf]): an inline struct's dominant fields sit at the parent level —
// a field shadowed by a shallower one of the same name yields no path — and
// an embedded struct without inline is a subdocument. A stripped inline
// struct denies each of its dominant keys.
//
// A slice element adds no path segment: MongoDB applies "items.secret" to
// every element of the array. A map value does — its key — so a stripped
// field inside map values cannot be addressed and the whole map is denied
// instead; no path under a map is listed as selectable. An inline map has
// dynamic keys at the parent level and yields no path; stripped data in one
// is [ErrStrippedInline].
func (o *options) collectPaths(obj behavior.Object, prefix string, ps *pathSet) error {
	if !obj.Value.IsValid() {
		// The synthetic wrapper of a nested-collection element: one unnamed
		// field under the prefix.
		for i := range obj.Fields {
			if err := o.collectField(&obj.Fields[i], prefix, false, ps); err != nil {
				return err
			}
		}
		return nil
	}

	layout, err := o.objectLayout(obj)
	if err != nil {
		return err
	}
	for _, df := range layout.fields {
		if err := checkPathName(df.name); err != nil {
			return err
		}
		path := joinPath(prefix, df.name)
		f, exact, ok := resolveField(obj, df)
		switch {
		case ok && !exact:
			// A stripped inline ancestor: this key is stripped with it.
			ps.denied = append(ps.denied, path)
			continue
		case !ok:
			continue
		}
		if err := o.collectField(f, path, true, ps); err != nil {
			return err
		}
	}

	if layout.inlineMap >= 0 {
		loc := locate(obj, []int{layout.inlineMap})
		if f := loc.field; f != nil && loc.exact {
			if f.Strip {
				return fmt.Errorf("%w: %s", ErrStrippedInline, f.Name)
			}
			if c := f.Collection; c != nil && len(c.Items) > 0 {
				strips, err := o.containsStrip(c.Items[0])
				if err != nil {
					return err
				}
				if strips {
					return fmt.Errorf("%w: %s", ErrStrippedInline, f.Name)
				}
			}
		}
	}
	return nil
}

// collectField records f under path — denied when stripped, otherwise
// selectable when named — and descends into its nested struct or collection.
func (o *options) collectField(f *behavior.Field, path string, named bool, ps *pathSet) error {
	if f.Strip {
		ps.denied = append(ps.denied, path)
		return nil
	}
	if named {
		ps.all = append(ps.all, path)
	}
	if f.Value.IsValid() && leafLayer(f.Value.Type()) {
		// Encoded as a unit (bson.Binary, a marshaler, …), directly or as the
		// element of its pointer and collection layers: no document paths
		// below it. A pointer-receiver marshaler is skipped for a value the
		// codec cannot address, which then stores its fields; when those
		// carry stripped data the whole field is denied rather than guessed.
		strips, err := o.resolvedStrips(f)
		if err != nil {
			return err
		}
		if strips {
			ps.denied = append(ps.denied, path)
		}
		return nil
	}
	switch c := f.Collection; {
	case f.Nested != nil:
		return o.collectPaths(*f.Nested, path, ps)
	case c == nil || len(c.Items) == 0:
		return nil
	case hasMapLayer(f.Value):
		strips, err := o.containsStrip(c.Items[0])
		if err != nil {
			return err
		}
		if strips {
			ps.denied = append(ps.denied, path)
		}
		return nil
	default:
		return o.collectPaths(c.Items[0], path, ps)
	}
}

// resolvedStrips reports whether the resolved shape below f — its nested
// object or its first collection element — holds a stripped field.
func (o *options) resolvedStrips(f *behavior.Field) (bool, error) {
	switch {
	case f.Nested != nil:
		return o.containsStrip(*f.Nested)
	case f.Collection != nil && len(f.Collection.Items) > 0:
		return o.containsStrip(f.Collection.Items[0])
	default:
		return false, nil
	}
}

// containsStrip reports whether obj holds a stripped field at any depth.
func (o *options) containsStrip(obj behavior.Object) (bool, error) {
	var sub pathSet
	if err := o.collectPaths(obj, "", &sub); err != nil {
		return false, err
	}
	return len(sub.denied) > 0, nil
}

// leafLayer reports whether t, or a type reached from it through pointer,
// slice, array and map layers, is a codec leaf (see isCodecLeafType).
func leafLayer(t reflect.Type) bool {
	for range maxCollectionLayers {
		if isCodecLeafType(t) {
			return true
		}
		switch t.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
			t = t.Elem()
		default:
			return false
		}
	}
	return false
}

// hasMapLayer reports whether the declared type of v reaches its element
// through a map at any collection layer ([]map[string]T, map[string][]T, …).
// The resolved tree flattens nested collection layers down to the struct
// element, so the layers are read off the type.
func hasMapLayer(v reflect.Value) bool {
	if !v.IsValid() {
		return false
	}
	// The bound guards self-referential collection types (type L []L).
	// Running out of layers fails closed: the caller then treats the field
	// as a map and denies it whole when its elements carry stripped data,
	// rather than emitting a path that may sit under a map key.
	t := v.Type()
	for range maxCollectionLayers {
		switch t.Kind() {
		case reflect.Map:
			return true
		case reflect.Pointer, reflect.Slice, reflect.Array:
			t = t.Elem()
		default:
			return false
		}
	}
	return true
}

// maxCollectionLayers bounds the type unwrapping in [hasMapLayer].
const maxCollectionLayers = 32

// hasInline reports whether a bson tag carries the inline option, in any
// token as the codec reads it.
func hasInline(tag string) bool {
	return bsonTag(tag, "x").inline
}
