// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/domain/behavior"
)

// mode selects which document a translation produces.
type mode uint8

const (
	modeInsert mode = iota
	modeUpdate
)

// foldObject builds the set/unset maps for a single resolved object. On update,
// fields the engine marked Strip (via behavior.WithKinds on the engine) are
// excluded from both operators. It applies at every nesting level: nested
// objects (struct pointers, slice elements, map values) all come from the
// engine's walk and carry their own Strip marks.
func foldObject(obj behavior.Object, o *options, m mode) (bson.M, bson.M, error) {
	set := make(bson.M, len(obj.Fields))
	// $unset entries are written only on update; insert leaves the map nil so
	// every object (including each nested subdocument) skips the allocation.
	var unset bson.M
	if m == modeUpdate {
		unset = bson.M{}
	}

	for i := range obj.Fields {
		f := &obj.Fields[i]

		name, omitEmpty, omitOnUpdate := parseBSONTag(f.Tag.Get(o.bsonTagName), f.Name)
		if name == "" {
			continue
		}
		if m == modeUpdate && (omitOnUpdate || f.Strip) {
			continue
		}

		if err := foldField(f, name, omitEmpty, o, m, set, unset); err != nil {
			return nil, nil, err
		}
	}

	return set, unset, nil
}

// foldField writes a single field into set/unset.
func foldField(f *behavior.Field, name string, omitEmpty bool, o *options, m mode, set, unset bson.M) error {
	fv := f.Value

	// Pre-processing filters.
	if m == modeUpdate && fv.Kind() == reflect.Pointer && fv.IsNil() {
		unset[name] = nil
		return nil
	}
	// omitempty matches the BSON driver: a zero value (zero scalar, nil pointer,
	// zero struct) is omitted on insert. A non-nil pointer is not zero, even
	// when it points at a zero value. Update ignores omitempty entirely.
	if m == modeInsert && omitEmpty && fv.IsZero() {
		return nil
	}
	if isEmptyCollection(fv) {
		if m == modeUpdate {
			unset[name] = nil
		}
		return nil
	}

	// Type dispatch.
	switch {
	case fv.Kind() == reflect.Bool:
		set[name] = fv.Bool()
		return nil

	case fv.Kind() == reflect.Slice && fv.Len() > 0 && !isBytesSlice(fv):
		arr, err := foldSlice(f, o, m)
		if err != nil {
			return err
		}
		set[name] = arr
		return nil

	case isStructPointerField(fv):
		sub, _, err := foldObject(*f.Nested, o, m)
		if err != nil {
			return err
		}
		set[name] = sub
		return nil

	case fv.Kind() == reflect.Map && fv.Len() > 0:
		sub, err := foldMap(f, name, o, m)
		if err != nil {
			return err
		}
		set[name] = sub
		return nil

	default:
		set[name] = fv.Interface()
		return nil
	}
}

// foldSlice converts a non-empty, non-[]byte slice. Struct elements that should
// be recursed (see recurseIntoStruct) become subdocuments; every other element —
// including opaque-leaf structs such as time.Time and custom BSON marshalers — is
// written opaquely via Interface(). f.Collection.Items is index-aligned with the
// slice for recursable struct elements.
func foldSlice(f *behavior.Field, o *options, m mode) (bson.A, error) {
	fv := f.Value
	out := make(bson.A, 0, fv.Len())

	for i := range fv.Len() {
		elem := reflect.Indirect(fv.Index(i))
		if elem.Kind() != reflect.Struct || !recurseIntoStruct(elem.Type()) {
			out = append(out, fv.Index(i).Interface())
			continue
		}

		// f.Collection is populated by the behavior walk for recursable struct
		// elements and its Items are index-aligned with the slice.
		sub, _, err := foldObject(f.Collection.Items[i], o, m)
		if err != nil {
			return nil, err
		}
		out = append(out, sub)
	}

	return out, nil
}

// foldMap converts a non-empty map with string keys. Keys starting with "$" are
// rejected so a map[string]any cannot smuggle MongoDB update operators into the
// document. Struct values that should be recursed become subdocuments folded
// from the engine's resolved Items — behavior strip marks apply to them exactly
// as to slice elements and nested struct pointers; all other values are written
// opaquely via Interface().
func foldMap(f *behavior.Field, name string, o *options, m mode) (bson.M, error) {
	fv := f.Value
	out := make(bson.M, fv.Len())

	// Collection.Keys/Items are parallel; index the engine's resolved entries
	// by key so the random map iteration order below can correlate them.
	var resolved map[string]int
	if c := f.Collection; c != nil && len(c.Keys) > 0 {
		resolved = make(map[string]int, len(c.Keys))
		for j, k := range c.Keys {
			if k.Kind() == reflect.String {
				resolved[k.String()] = j
			}
		}
	}

	iter := fv.MapRange()
	for iter.Next() {
		k := iter.Key()
		if k.Kind() != reflect.String {
			return nil, fmt.Errorf("behavior/mongo: %s: map key must be string", name)
		}
		key := k.String()
		if strings.HasPrefix(key, "$") {
			return nil, fmt.Errorf("behavior/mongo: %s: map key %q must not start with %q (reserved for MongoDB operators)", name, key, "$")
		}

		val := iter.Value()
		elem := reflect.Indirect(val)
		if elem.Kind() == reflect.Struct && recurseIntoStruct(elem.Type()) {
			j, ok := resolved[key]
			if !ok {
				// A recursable struct value the engine did not resolve would mean
				// the walk and the translator disagree on the field's shape.
				return nil, fmt.Errorf("behavior/mongo: %s: no resolved object for map key %q", name, key)
			}
			sub, _, err := foldObject(f.Collection.Items[j], o, m)
			if err != nil {
				return nil, err
			}
			out[key] = sub
			continue
		}
		out[key] = val.Interface()
	}

	return out, nil
}

// isStructPointerField reports whether fv is a non-nil pointer to a recursable
// struct, mirroring the legacy isStructPointerField guard.
func isStructPointerField(fv reflect.Value) bool {
	if fv.Kind() != reflect.Pointer || fv.IsNil() {
		return false
	}
	elem := fv.Type().Elem()
	return elem.Kind() == reflect.Struct && recurseIntoStruct(elem)
}

// isEmptyCollection reports whether fv is an empty slice, map, or array.
func isEmptyCollection(fv reflect.Value) bool {
	switch fv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array:
		return fv.Len() == 0
	default:
		return false
	}
}

// isBytesSlice reports whether fv is a []byte / []uint8, which the BSON driver
// encodes as Binary rather than as an array of elements.
func isBytesSlice(fv reflect.Value) bool {
	return fv.Kind() == reflect.Slice && fv.Type().Elem().Kind() == reflect.Uint8
}

// BSON marshaling interface types, resolved once for the recurseIntoStruct guard.
var (
	bsonMarshalerType      = reflect.TypeFor[bson.Marshaler]()
	bsonValueMarshalerType = reflect.TypeFor[bson.ValueMarshaler]()
)

// structRecursionCache memoizes recurseIntoStruct decisions. The set of struct
// types encountered during conversion is finite, so an unbounded sync.Map is
// sufficient and avoids the per-element reflection scan on slice/map walks.
var structRecursionCache sync.Map // map[reflect.Type]bool

// recurseIntoStruct reports whether a struct of type t should be walked
// field-by-field. A struct is walked only when it exposes exported fields AND
// does not define its own BSON serialization (bson.Marshaler / bson.ValueMarshaler).
// Opaque structs such as time.Time and self-marshaling types are scalar leaves.
func recurseIntoStruct(t reflect.Type) bool {
	if cached, ok := structRecursionCache.Load(t); ok {
		walk, _ := cached.(bool)
		return walk
	}
	walk := hasExportedField(t) && !implementsBSONMarshaler(t)
	structRecursionCache.Store(t, walk)
	return walk
}

// hasExportedField reports whether t has at least one exported field.
func hasExportedField(t reflect.Type) bool {
	for i := range t.NumField() {
		if t.Field(i).IsExported() {
			return true
		}
	}
	return false
}

// implementsBSONMarshaler reports whether t or *t implements bson.Marshaler or
// bson.ValueMarshaler. The pointer method set is a superset of the value method
// set, so checking *t covers both receiver styles.
func implementsBSONMarshaler(t reflect.Type) bool {
	ptr := reflect.PointerTo(t)
	return ptr.Implements(bsonMarshalerType) || ptr.Implements(bsonValueMarshalerType)
}

// bsonTagInfo is the parsed form of one bson tag, memoized in bsonTagCache.
type bsonTagInfo struct {
	name         string
	omitEmpty    bool
	omitOnUpdate bool
}

// bsonTagKey identifies a parse: the Go field name participates because it is
// the fallback document name when the tag names no field.
type bsonTagKey struct{ tag, goName string }

// bsonTagCache memoizes parseBSONTag results. Tags and field names come from
// struct definitions, so the key space is finite and an unbounded sync.Map is
// sufficient; the cache removes a per-field strings.Split on every Translate.
var bsonTagCache sync.Map // map[bsonTagKey]bsonTagInfo

// parseBSONTag extracts the document field name and modifiers from a bson tag,
// falling back to the lowercased Go field name when the tag is absent or names no
// field. A "-" or empty tag name yields an empty fieldName (skip).
func parseBSONTag(tag, goName string) (fieldName string, omitEmpty, omitOnUpdate bool) {
	key := bsonTagKey{tag: tag, goName: goName}
	if v, ok := bsonTagCache.Load(key); ok {
		info, _ := v.(bsonTagInfo)
		return info.name, info.omitEmpty, info.omitOnUpdate
	}

	var info bsonTagInfo
	parts := strings.Split(tag, ",")
	if info.name = parts[0]; info.name != "-" {
		for _, p := range parts[1:] {
			switch p {
			case "omitempty":
				info.omitEmpty = true
			case "omitonupdate":
				info.omitOnUpdate = true
			}
		}
		if info.name == "" {
			info.name = strings.ToLower(goName)
		}
	} else {
		info.name = ""
	}

	bsonTagCache.Store(key, info)
	return info.name, info.omitEmpty, info.omitOnUpdate
}

// docTranslator folds a resolved behavior Object into an insert or update BSON
// document. It implements [behavior.Translator] so the behavior engine's single
// walk feeds this output model; build it through [NewInsertTranslator] /
// [NewUpdateTranslator] and drive it with [github.com/altessa-s/go-atlas/domain/behavior.New].
//
// A docTranslator holds no per-call state and is safe for concurrent use: every
// Translate call accumulates into fresh maps.
type docTranslator struct {
	o    *options
	mode mode
}

// NewInsertTranslator returns a [behavior.Translator] producing insert documents
// under opts. It emits every exported field (bson name, omitempty, []byte binary,
// nested/opaque-leaf as today) and applies no behavior-kind filtering — drive it
// through [github.com/altessa-s/go-atlas/domain/behavior.New] with no
// behavior.WithKinds so nothing is marked Strip.
func NewInsertTranslator(opts ...Option) behavior.Translator[bson.M] {
	return &docTranslator{o: newOptions(opts...), mode: modeInsert}
}

// NewUpdateTranslator returns a [behavior.Translator] producing update documents
// ({"$set": …, "$unset": …}) under opts. "_id" is removed from "$set"; nil
// pointers and empty collections of a non-stripped field go to "$unset"; nested
// values are full-replaced in "$set". Fields the engine marked Strip are excluded
// from both operators — recursively, so nested subdocuments built from struct
// pointers, slice elements, and map values all drop them — drive it through
// [github.com/altessa-s/go-atlas/domain/behavior.New] with
// behavior.WithKinds(behavior.DefaultUpdateKinds...) to exclude server-owned and
// immutable fields.
func NewUpdateTranslator(opts ...Option) behavior.Translator[bson.M] {
	return &docTranslator{o: newOptions(opts...), mode: modeUpdate}
}

// Translate folds root into a BSON document for the translator's mode.
func (t *docTranslator) Translate(_ context.Context, root behavior.Object) (bson.M, error) {
	if t.mode == modeUpdate {
		set, unset, err := foldObject(root, t.o, modeUpdate)
		if err != nil {
			return nil, err
		}
		delete(set, "_id")
		result := bson.M{"$set": set}
		if len(unset) > 0 {
			result["$unset"] = unset
		}
		return result, nil
	}

	set, _, err := foldObject(root, t.o, modeInsert)
	if err != nil {
		return nil, err
	}
	return set, nil
}
