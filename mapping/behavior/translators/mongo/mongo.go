// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-convert/reflectx"

	"github.com/altessa-s/go-atlas/mapping/behavior"
)

// mode selects which document a translation produces.
type mode uint8

const (
	modeInsert mode = iota
	modeUpdate
)

// encCtx is the encoding context folding passes down, mirroring what the
// codec passes to nested encoders.
type encCtx struct {
	// minSize is the inherited minsize option (struct_codec.go: a field's own
	// option or its parent's).
	minSize bool
	// noAddr marks values the codec reaches without addressability — the
	// values of a map — although the engine resolved them through addressable
	// copies, so pointer-receiver marshalers stay unused, as in the codec.
	noAddr bool
	// reg is the configured registry, nil for the driver's default.
	reg *bson.Registry
	// inColl marks values folded as the elements of a collection (slice,
	// array or map values): their identifiers are the elements' own and are
	// written with them, and a stripped field among them is an error, see
	// [excludeOnUpdate].
	inColl bool
	// addressed marks an object whose keys become update operator paths (the
	// root, and a nested object written by descendant paths): each key must be
	// addressable (see checkPathName).
	addressed bool
	// root marks the fields of the root document, inline contributions
	// included: its "_id" is immutable in MongoDB and never written.
	root bool
}

// elementDoc resets the document-position flags for the values of a
// collection — slice and array elements, map values, the values of a root
// inline map — which are documents of their own: not the root (their _id is
// theirs and is written) and not addressed (their keys are stored literally,
// not read as operator paths). The inline map's own keys keep the parent's
// flags; foldInlineMap checks them.
func (ec *encCtx) elementDoc() {
	ec.root = false
	ec.addressed = false
}

// foldObject builds the set/unset maps for a single resolved object, laid out
// as the BSON struct codec lays out the object's struct type (see
// [layoutOf]): inline struct fields sit at the parent level, an embedded
// struct without inline is a subdocument, and an inline map's entries join
// the parent. On update, fields the engine marked Strip (via
// behavior.WithKinds on the engine) and identifiers are excluded from both
// operators (see [excludeOnUpdate]). It applies at every nesting level: nested
// objects (struct pointers, slice elements, map values) all come from the
// engine's walk and carry their own Strip marks.
func foldObject(obj behavior.Object, o *options, m mode, ec encCtx) (bson.M, bson.M, error) {
	layout, err := o.objectLayout(obj)
	if err != nil {
		return nil, nil, err
	}
	set := make(bson.M, len(layout.fields))
	// $unset entries are written only on update; insert leaves the map nil so
	// every object (including each nested subdocument) skips the allocation.
	var unset bson.M
	if m == modeUpdate {
		unset = bson.M{}
	}

	for _, df := range layout.fields {
		if err := foldLayoutField(obj, df, o, m, ec, set, unset); err != nil {
			return nil, nil, err
		}
	}

	if layout.inlineMap >= 0 {
		if err := foldInlineMap(obj, layout, o, m, ec, set); err != nil {
			return nil, nil, err
		}
	}
	return set, unset, nil
}

// foldLayoutField folds the layout key df of obj into set and unset, or
// leaves it out: on update an omitonupdate key outside a collection, the root
// _id, a key behind a nil pointer or a stripped inline ancestor, and an
// excluded or identifier-governed field are not written.
func foldLayoutField(obj behavior.Object, df docField, o *options, m mode, ec encCtx, set, unset bson.M) error {
	// An omitonupdate field is never written. Outside a collection its
	// stored value survives; inside a replaced collection element it goes
	// with the element, so a protected one fails below instead of being
	// dropped.
	omit := m == modeUpdate && df.omitOnUpdate
	if omit && !ec.inColl {
		return nil
	}
	if m == modeUpdate && ec.root && df.name == "_id" {
		// MongoDB rejects any change to the _id subtree: neither the
		// field nor a descendant path of a composite _id is written.
		return nil
	}
	f, ok, err := resolveFoldField(obj, df, o, m, ec)
	if err != nil || !ok {
		return err
	}
	if m == modeUpdate {
		skip, err := skipOnUpdate(obj, df, f, ec)
		if err != nil || skip {
			return err
		}
	}
	if omit {
		return checkOmittedInCollection(obj, df, f, o, ec)
	}
	fieldCtx := ec
	if crossesPointer(obj.Value.Type(), df.index) {
		// Reached through an inline pointer: addressable to the codec.
		fieldCtx.noAddr = false
	}
	return foldField(f, df, o, m, fieldCtx, set, unset)
}

// resolveFoldField returns the field to fold for the layout key df. It
// reports false when the key is not written, and an error when a replaced
// collection element would lose a protected stored value.
func resolveFoldField(obj behavior.Object, df docField, o *options, m mode, ec encCtx) (*behavior.Field, bool, error) {
	f, exact, ok := resolveField(obj, df)
	switch {
	case ok && !exact:
		return strippedAncestorField(obj, df, f, m, ec)
	case !ok:
		// Behind a nil pointer: the codec writes no key. A replaced
		// collection element then loses the stored key, so a protected
		// one fails (see [options.protectedType]).
		if m == modeUpdate && ec.inColl && o.protectedField(obj.Value.Type(), df, true, ec.noAddr, o.policy, nil) {
			return nil, false, fmt.Errorf("%w: %s", ErrProtectedInCollection, df.goName)
		}
		return nil, false, nil
	default:
		return f, true, nil
	}
}

// strippedAncestorField handles a key governed by the stripped inline
// ancestor f. Update drops it — or fails inside a replaced collection, which
// cannot preserve it: even a stripped identifier ancestor left its children
// unresolved, so their behaviors cannot be checked. Insert is unfiltered and
// writes the stored value as is.
func strippedAncestorField(obj behavior.Object, df docField, f *behavior.Field, m mode, ec encCtx) (*behavior.Field, bool, error) {
	if m == modeUpdate {
		if !ec.inColl {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("%w: %s", ErrProtectedInCollection, f.Name)
	}
	fv, err := obj.Value.FieldByIndexErr(df.index)
	if err != nil {
		return nil, false, nil //nolint:nilerr // Behind a nil inline pointer: the codec writes no key.
	}
	return &behavior.Field{Index: df.index, Name: df.goName, Tag: df.tag, Value: fv}, true, nil
}

// skipOnUpdate reports whether the update document leaves the resolved field
// f out: it was contributed by an inline struct tagged identifier — the
// identifier covers every key it adds to the parent — or [excludeOnUpdate]
// excludes it.
func skipOnUpdate(obj behavior.Object, df docField, f *behavior.Field, ec encCtx) (bool, error) {
	if !ec.inColl && len(df.index) > 1 && identifierAncestor(obj, df.index) {
		return true, nil
	}
	return excludeOnUpdate(f, ec)
}

// checkOmittedInCollection fails an omitonupdate field inside a replaced
// collection element when a protected field sits below it; the field's own
// strip mark already failed in [excludeOnUpdate].
func checkOmittedInCollection(obj behavior.Object, df docField, f *behavior.Field, o *options, ec encCtx) error {
	noAddr := (ec.noAddr && !crossesPointer(obj.Value.Type(), df.index)) || !f.Value.CanAddr()
	if o.protectedType(f.Value.Type(), true, noAddr, o.policy) {
		return fmt.Errorf("%w: %s", ErrProtectedInCollection, f.Name)
	}
	return nil
}

// excludeOnUpdate reports whether the update document leaves f out: the
// engine stripped it, or it is an identifier of the record or of a subdocument
// (AIP-203). An identifier selects the stored record and is never written,
// whatever kinds the engine was configured to strip — the translator excludes
// it itself, so behavior.DefaultUpdateKinds, which keeps the identifier in the
// payload, builds a safe update document.
//
// Inside a collection element, which the update replaces with the whole
// collection, nothing can be preserved by leaving it out: an identifier
// belongs to the element and is written with it. A field the engine stripped
// is [ErrProtectedInCollection], except an identifier stripped for that reason
// alone (no other kind) whose value is written as a unit: a stripped compound
// identifier left its children unresolved, so they cannot be checked.
func excludeOnUpdate(f *behavior.Field, ec encCtx) (bool, error) {
	ident := isIdentifier(f)
	if !ec.inColl {
		return f.Strip || ident, nil
	}
	if !f.Strip {
		return false, nil
	}
	// The element-identifier exemption: stripped only for being an
	// identifier, and nothing below it the engine left unresolved.
	if ident && onlyIdentifier(f.Kinds) && !isCompound(f.Value, ec) {
		return false, nil
	}
	return false, fmt.Errorf("%w: %s", ErrProtectedInCollection, f.Name)
}

// onlyIdentifier reports whether kinds holds Identifier and no other kind, so
// a strip mark on the field can only stem from the identifier.
func onlyIdentifier(kinds []behavior.Kind) bool {
	for _, k := range kinds {
		if k != behavior.Identifier {
			return false
		}
	}
	return len(kinds) > 0
}

// isCompound reports whether fv, through non-nil pointers, is folded rather
// than written as a unit: a struct, slice, array or map with parts the
// engine would resolve. A stripped field is a leaf to the engine, so such a
// value carries children whose behaviors were never checked.
func isCompound(fv reflect.Value, ec encCtx) bool {
	for fv.IsValid() && !isCodecLeaf(fv, ec) {
		switch fv.Kind() {
		case reflect.Pointer:
			if fv.IsNil() {
				return false
			}
			fv = fv.Elem()
			ec.noAddr = false
		case reflect.Struct:
			return recurseIntoStruct(fv.Type()) || (pointerOnlyMarshaler(fv.Type()) && hasExportedField(fv.Type()))
		case reflect.Slice, reflect.Array, reflect.Map:
			return true
		default:
			return false
		}
	}
	return false
}

// identifierAncestor reports whether a resolved field on the Go index path
// index above its last element — an inline struct the field is imported
// through — is tagged identifier. The engine does not strip identifiers under
// behavior.DefaultUpdateKinds, so it descends into such a struct and resolves
// its untagged fields; the tag on the inline ancestor still governs them.
func identifierAncestor(obj behavior.Object, index []int) bool {
	for i := range obj.Fields {
		fi := &obj.Fields[i]
		if len(fi.Index) >= len(index) || !slices.Equal(fi.Index, index[:len(fi.Index)]) {
			continue
		}
		if isIdentifier(fi) {
			return true
		}
		if fi.Nested != nil {
			return identifierAncestor(*fi.Nested, index[len(fi.Index):])
		}
		return false
	}
	return false
}

// isIdentifier reports whether f is tagged identifier.
func isIdentifier(f *behavior.Field) bool {
	return slices.Contains(f.Kinds, behavior.Identifier)
}

// pathObject returns the resolved object foldValue would fold fv into a
// subdocument — through any non-nil pointers — and the context it folds it
// under, or nil when fv is not written as a folded subdocument (a codec leaf,
// a collection, a scalar).
func pathObject(fv reflect.Value, nested *behavior.Object, ec encCtx) (*behavior.Object, encCtx) {
	for nested != nil && fv.IsValid() && !isCodecLeaf(fv, ec) {
		switch fv.Kind() {
		case reflect.Pointer:
			if fv.IsNil() {
				return nil, ec
			}
			fv = fv.Elem()
			ec.noAddr = false
		case reflect.Struct:
			if recurseIntoStruct(fv.Type()) || (pointerOnlyMarshaler(fv.Type()) && hasExportedField(fv.Type())) {
				return nested, ec
			}
			return nil, ec
		default:
			return nil, ec
		}
	}
	return nil, ec
}

// foldInlineMap merges the entries of the object's inline map into its parent
// document. A key that names any field of the layout is an error, as in the
// codec, whether or not that field holds a value. A nil or empty map, or one
// stripped on update, contributes nothing: its keys are dynamic, so there is
// no stored name to $unset.
func foldInlineMap(obj behavior.Object, layout *docLayout, o *options, m mode, ec encCtx, set bson.M) error {
	loc := locate(obj, []int{layout.inlineMap})
	f, exact, ok := loc.field, loc.exact, loc.field != nil
	// An inline map tagged identifier follows excludeOnUpdate: left out of
	// the record, written with a collection element unless stripped — a
	// stripped map is a leaf to the engine, its values never inspected.
	if ok && m == modeUpdate {
		switch {
		case !ec.inColl && isIdentifier(f):
			return nil
		case ec.inColl && (!exact || f.Strip):
			return fmt.Errorf("%w: %s", ErrProtectedInCollection, f.Name)
		}
	}
	if !ok || !exact || (m == modeUpdate && f.Strip) {
		return nil
	}
	omit := m == modeUpdate && hasOmitOnUpdate(o.fieldTag(f.Tag))
	// A replaced collection element loses the stored entries of an empty or
	// omitted inline map with it.
	if m == modeUpdate && ec.inColl && (omit || f.Value.Len() == 0) && o.protectedType(f.Value.Type(), true, ec.noAddr, o.policy) {
		return fmt.Errorf("%w: %s", ErrProtectedInCollection, f.Name)
	}
	if omit || f.Value.Len() == 0 {
		return nil
	}
	// The codec encodes an inline map under the enclosing context: the map
	// field's own minsize tag has no effect.
	sub, err := foldMap(f, f.Name, o, m, ec)
	if err != nil {
		return err
	}
	for k, v := range sub {
		if _, clash := layout.names[k]; clash {
			return fmt.Errorf("behavior/mongo: %s: inline map key %q conflicts with a struct field name", f.Name, k)
		}
		if m == modeUpdate && ec.addressed {
			if err := checkPathName(k); err != nil {
				return err
			}
		}
		set[k] = v
	}
	return nil
}

// hasOmitOnUpdate reports whether a bson tag carries the omitonupdate option.
func hasOmitOnUpdate(tag string) bool {
	_, _, omitOnUpdate := parseBSONTag(tag, "x")
	return omitOnUpdate
}

// foldField writes a single field into set/unset.
func foldField(f *behavior.Field, df docField, o *options, m mode, parent encCtx, set, unset bson.M) error {
	fv := f.Value
	name, omitEmpty := df.name, df.omitEmpty
	// minsize reaches every value under the field, as the codec passes it down
	// through pointers, collections and nested structs.
	ec := parent
	ec.minSize = ec.minSize || df.minSize

	// Insert follows the codec: omitempty drops what the codec deems empty
	// (see isCodecEmpty), and an empty or nil collection without omitempty is
	// written — [] / {} or null. Update ignores omitempty, unsets a nil
	// pointer and an empty collection, and fails instead when its declared
	// type holds a protected field the unset would delete (see
	// [options.protectedType]).
	if m == modeInsert && omitEmpty && isCodecEmpty(fv) {
		return nil
	}
	if m == modeUpdate && ec.addressed {
		// The key is an operator path: a dot or a "$" prefix in the stored
		// name would address another field.
		if err := checkPathName(name); err != nil {
			return err
		}
	}
	if m == modeUpdate && isRemoval(fv) {
		if o.protectedType(fv.Type(), ec.inColl, ec.noAddr || !fv.CanAddr(), o.policy) {
			return fmt.Errorf("%w: %s", removalError(fv.Type(), ec), name)
		}
		unset[name] = nil
		return nil
	}
	if m == modeUpdate && ec.addressed && !ec.inColl {
		if obj, objCtx := pathObject(fv, f.Nested, ec); obj != nil && o.holdsProtected(*obj, objCtx) {
			return foldPaths(*obj, name, o, objCtx, set, unset)
		}
	}

	ec.addressed = false
	ec.root = false
	v, err := foldValue(fv, f.Nested, f.Collection, o, m, ec)
	if err != nil {
		return fmt.Errorf("behavior/mongo: %s: %w", name, err)
	}
	set[name] = v
	return nil
}

// foldPaths writes the nested object obj of the field stored as name by
// descendant paths ("name.child") instead of replacing the subdocument:
// obj holds a field the update leaves out, and a replacement would delete its
// stored value. Every other field below is set or unset by its own path, so
// the result equals a replacement that keeps the protected fields; an object
// holding nothing else contributes no key at all.
func foldPaths(obj behavior.Object, name string, o *options, ec encCtx, set, unset bson.M) error {
	ec.addressed = true
	ec.root = false
	subSet, subUnset, err := foldObject(obj, o, modeUpdate, ec)
	if err != nil {
		return fmt.Errorf("behavior/mongo: %s: %w", name, err)
	}
	for k, v := range subSet {
		set[name+"."+k] = v
	}
	for k := range subUnset {
		unset[name+"."+k] = nil
	}
	return nil
}

// foldValue folds fv along the shape the engine resolved for it — nested for
// a struct or pointer-to-struct, coll for a slice, array or map, through any
// number of pointer and collection layers — so strip marks and omitonupdate
// apply at every depth. A value the codec encodes as a unit (see isCodecLeaf),
// an interface, or anything the engine did not resolve is encoded as the
// codec would (see encodeLeaf). Addressability follows the codec: a pointer's
// target and a slice's elements are addressable, a map's values are not, an
// array's elements are when the array is.
func foldValue(fv reflect.Value, nested *behavior.Object, coll *behavior.Collection, o *options, m mode, ec encCtx) (any, error) {
	if !fv.IsValid() {
		return nil, nil //nolint:nilnil // a nil interface element is stored as null
	}
	if m == modeUpdate && isRemoval(fv) && o.protectedType(fv.Type(), ec.inColl, ec.noAddr || !fv.CanAddr(), o.policy) {
		// A null or an empty collection in place of a stored value that may
		// hold protected fields: a nil element, a nil or empty nested
		// collection.
		return nil, fmt.Errorf("%w: %s", removalError(fv.Type(), ec), fv.Type())
	}
	if isCodecLeaf(fv, ec) {
		return encodeLeaf(fv, ec)
	}
	switch fv.Kind() {
	case reflect.Pointer:
		if fv.IsNil() {
			return nil, nil //nolint:nilnil // the codec writes a nil pointer as null
		}
		inner := ec
		inner.noAddr = false
		return foldValue(fv.Elem(), nested, coll, o, m, inner)
	case reflect.Struct:
		// A struct whose marshaler the codec does not select here (see
		// isCodecLeaf) is encoded field by field and folded like any other.
		if nested != nil && (recurseIntoStruct(fv.Type()) || (pointerOnlyMarshaler(fv.Type()) && hasExportedField(fv.Type()))) {
			sub, _, err := foldObject(*nested, o, m, ec)
			return sub, err
		}
	case reflect.Slice, reflect.Array:
		if fv.Kind() == reflect.Array || !fv.IsNil() {
			return foldList(fv, coll, o, m, ec)
		}
	case reflect.Map:
		// Every map is folded, resolved elements or not, so its keys pass the
		// string and "$" checks.
		if !fv.IsNil() {
			return foldMapValue(fv, coll, o, m, ec)
		}
	default:
	}
	return encodeLeaf(fv, ec)
}

// foldList folds the elements of a slice or array; coll may be nil when the
// engine resolved none (scalar elements).
func foldList(fv reflect.Value, coll *behavior.Collection, o *options, m mode, parent encCtx) (bson.A, error) {
	ec := parent
	ec.inColl = true
	ec.elementDoc()
	if fv.Kind() == reflect.Slice {
		// A slice's elements live in its backing array: addressable to the
		// codec even when the slice itself was reached through a map value.
		ec.noAddr = false
	}
	out := make(bson.A, fv.Len())
	for i := range fv.Len() {
		v, err := foldElement(fv.Index(i), coll, i, o, m, ec)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// foldElement folds element i of a collection with its resolved Items entry:
// a struct's Object, the synthetic wrapper of a nested collection, or an
// empty placeholder for anything else.
func foldElement(ev reflect.Value, coll *behavior.Collection, i int, o *options, m mode, ec encCtx) (any, error) {
	if coll == nil || i < 0 || i >= len(coll.Items) {
		// Unresolved — scalar at the bottom — yet possibly a nested map whose
		// keys still need checking.
		return foldValue(ev, nil, nil, o, m, ec)
	}
	item := coll.Items[i]
	switch {
	case item.Value.IsValid():
		return foldValue(ev, &item, nil, o, m, ec)
	case len(item.Fields) == 1 && item.Fields[0].Name == "":
		w := item.Fields[0]
		return foldValue(ev, w.Nested, w.Collection, o, m, ec)
	default:
		return foldValue(ev, nil, nil, o, m, ec)
	}
}

// foldMap folds a map field's entries into a document, as [foldMapValue].
func foldMap(f *behavior.Field, name string, o *options, m mode, ec encCtx) (bson.M, error) {
	sub, err := foldMapValue(f.Value, f.Collection, o, m, ec)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return sub, nil
}

// foldMapValue folds a map with string keys into a document. Keys starting
// with "$" are rejected so a map[string]any cannot smuggle MongoDB update
// operators into the document. Map values are not addressable to the codec,
// so neither they nor anything folded from them use pointer-receiver
// marshalers, until a pointer is followed.
func foldMapValue(fv reflect.Value, coll *behavior.Collection, o *options, m mode, parent encCtx) (bson.M, error) {
	ec := parent
	ec.noAddr = true
	ec.inColl = true
	ec.elementDoc()

	// Collection.Keys/Items are parallel; index the engine's resolved entries
	// by key so the random map iteration order below can correlate them.
	resolved := make(map[string]int, fv.Len())
	if coll != nil {
		for j, k := range coll.Keys {
			if k.Kind() == reflect.String {
				resolved[k.String()] = j
			}
		}
	}

	out := make(bson.M, fv.Len())
	for iter := fv.MapRange(); iter.Next(); {
		k := iter.Key()
		if k.Kind() != reflect.String {
			return nil, fmt.Errorf("behavior/mongo: map key must be string")
		}
		key := k.String()
		if strings.HasPrefix(key, "$") {
			return nil, fmt.Errorf("behavior/mongo: map key %q must not start with %q (reserved for MongoDB operators)", key, "$")
		}
		j, ok := resolved[key]
		if !ok {
			j = -1
		}
		v, err := foldElement(iter.Value(), coll, j, o, m, ec)
		if err != nil {
			return nil, err
		}
		out[key] = v
	}
	return out, nil
}

// isRemoval reports whether fv, written on update, removes whatever is stored
// in its place: a nil pointer, or a nil or empty slice, map or array.
func isRemoval(fv reflect.Value) bool {
	switch fv.Kind() {
	case reflect.Pointer:
		return fv.IsNil()
	case reflect.Slice, reflect.Map, reflect.Array:
		return fv.Len() == 0
	default:
		return false
	}
}

// removalError returns the sentinel for removing a value of type t that may
// hold a protected field: [ErrProtectedInCollection] for a collection, or for
// anything inside a replaced collection element, [ErrProtectedUnset] for a
// subdocument.
func removalError(t reflect.Type, ec encCtx) error {
	t = reflectx.IndirectType(t)
	switch {
	case ec.inColl:
		return ErrProtectedInCollection
	case t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map:
		return ErrProtectedInCollection
	default:
		return ErrProtectedUnset
	}
}

// docTranslator folds a resolved behavior Object into an insert or update BSON
// document. It implements [behavior.Translator] so the behavior engine's single
// walk feeds this output model; build it through [NewInsertTranslator] /
// [NewUpdateTranslator] and drive it with [github.com/altessa-s/go-atlas/mapping/behavior.New].
//
// A docTranslator holds no per-call state and is safe for concurrent use: every
// Translate call accumulates into fresh maps.
type docTranslator struct {
	o    *options
	mode mode
}

// BindPolicy implements [behavior.PolicyBinder]: [behavior.New] calls it with
// the engine's policy, which the update translator applies to the values it
// judges from their declared type. t itself is left unchanged.
func (t *docTranslator) BindPolicy(p behavior.Policy) behavior.Translator[bson.M] {
	o := *t.o
	o.policy = p
	return &docTranslator{o: &o, mode: t.mode}
}

// NewInsertTranslator returns a [behavior.Translator] producing insert documents
// under opts. It emits every exported field (bson name, omitempty, []byte binary,
// nested/opaque-leaf as today) and applies no behavior-kind filtering — drive it
// through [github.com/altessa-s/go-atlas/mapping/behavior.New] with no
// behavior.WithKinds so nothing is marked Strip.
func NewInsertTranslator(opts ...Option) behavior.Translator[bson.M] {
	return &docTranslator{o: newOptions(opts...), mode: modeInsert}
}

// NewUpdateTranslator returns a [behavior.Translator] producing update documents
// ({"$set": …, "$unset": …}) under opts. "_id" is removed from both; nil
// pointers and empty collections of a non-stripped field go to "$unset"; nested
// values are full-replaced in "$set", except a nested object holding a field
// the update leaves out, which is written by descendant paths ("profile.name")
// so the stored value of that field survives. Fields the engine marked Strip
// are excluded from both operators — recursively through nested objects; a
// replaced collection whose elements carry one fails with
// [ErrProtectedInCollection], as the replacement cannot preserve it. A value
// that would remove stored data without resolved fields to check — a nil
// pointer, a nil or empty collection, a nil element — is judged from its
// declared type under the engine's own policy, which [behavior.New] binds to
// the translator (see [behavior.PolicyBinder]): when the type holds a field of
// a kind the engine strips, read under the engine's tag key (or, outside a
// collection, an identifier or omitonupdate field), the update fails with
// [ErrProtectedUnset] for a subdocument and [ErrProtectedInCollection] for a
// collection or inside one, instead of deleting the stored value. Populated
// and removed values thus follow the same policy.
// Drive it through
// [github.com/altessa-s/go-atlas/mapping/behavior.New] with
// behavior.WithKinds(behavior.DefaultUpdateKinds...) to exclude server-owned and
// immutable fields. Identifier fields are excluded by the translator itself,
// whatever kinds the engine strips: the identifier selects the record and is
// never written, under "_id" or any other name, set or nil. An identifier
// inside a collection element is the element's own and is written with it;
// stripped by the engine, only when it carries no other kind and its value is
// written as a unit (see excludeOnUpdate).
func NewUpdateTranslator(opts ...Option) behavior.Translator[bson.M] {
	return &docTranslator{o: newOptions(opts...), mode: modeUpdate}
}

// Translate folds root into a BSON document for the translator's mode.
func (t *docTranslator) Translate(_ context.Context, root behavior.Object) (bson.M, error) {
	if t.mode == modeUpdate {
		// The root's keys are operator paths: a field whose stored name holds
		// a dot (bson:"a.b", a literal key to the codec) would address another
		// field, and a "$" prefix is not a field at all, so folding checks
		// each one (see encCtx.addressed).
		set, unset, err := foldObject(root, t.o, modeUpdate, encCtx{reg: t.o.registry, addressed: true, root: true})
		if err != nil {
			return nil, err
		}
		// _id is immutable in MongoDB: neither operator may touch it. Folding
		// skips the root _id field (own or inline-contributed, with every
		// descendant path); an inline map entry named _id is dropped here.
		delete(set, "_id")
		delete(unset, "_id")
		result := bson.M{"$set": set}
		if len(unset) > 0 {
			result["$unset"] = unset
		}
		return result, nil
	}

	if t.o.bsonTagName == DefaultBSONTagName && root.Value.IsValid() {
		return encodeInsert(root.Value, t.o.registry)
	}
	set, _, err := foldObject(root, t.o, modeInsert, encCtx{reg: t.o.registry})
	if err != nil {
		return nil, err
	}
	return set, nil
}

// ErrUnaddressableName reports a stored field name an update operator or a
// projection path cannot address: it contains a dot, which MongoDB reads as a
// path separator although the codec stores the name literally, or it starts
// with "$". Rename the field's bson key.
var ErrUnaddressableName = errors.New("field name cannot be addressed by a document path")

// ErrProtectedInCollection reports an update that replaces a collection (a
// slice, array or map, at any depth) whose elements carry a field the engine
// marked Strip. The replacement would drop the stored values of that field and
// no document path can address it element by element, so the update is
// rejected instead. Tag the collection omitonupdate and update its elements
// with positional operators, or drop the kind from the engine's strip set.
var ErrProtectedInCollection = errors.New("stripped field inside a replaced collection cannot be preserved")

// ErrProtectedUnset reports an update that removes a subdocument — a nil
// pointer in place of a stored struct — whose declared type holds a protected
// field (see [NewUpdateTranslator]): unsetting it would delete the stored
// value. Send the subdocument instead, which is then written by descendant
// paths that keep the protected fields; leave the field out of the update
// (tag it omitonupdate); or, for a field protected only because the engine
// strips its kind, drop that kind from the engine's strip set. An empty or nil
// collection and a nil collection element fail with
// [ErrProtectedInCollection] instead.
var ErrProtectedUnset = errors.New("protected field inside a removed subdocument cannot be preserved")

// encodeInsert builds an insert document by encoding v with the BSON codec
// itself — an insert applies no behavior filtering, so the document is
// exactly what the driver would store for v, inline fields, omitempty,
// minsize and marshalers included — and decoding it into a bson.M (nested
// documents as ordered bson.D, arrays as bson.A, values as their BSON Go
// types). v is
// encoded through its address when it has one, as the driver encodes a
// pointer, with reg when it is non-nil. Keys starting with "$" are rejected at
// any depth, as on the folding path, so no map can smuggle an operator-like key
// into the document.
func encodeInsert(v reflect.Value, reg *bson.Registry) (bson.M, error) {
	// The codec recurses forever on a struct inlined into itself; reject what
	// layoutOf rejects before handing the value over.
	if !staticallySafe(v.Type()) {
		if err := validateValue(v, 0); err != nil {
			return nil, err
		}
	}
	target := v.Interface()
	if v.CanAddr() {
		target = v.Addr().Interface()
	}
	raw, err := marshal(target, reg)
	if err != nil {
		return nil, fmt.Errorf("behavior/mongo: encode insert document: %w", err)
	}
	if err := rejectOperatorKeys(raw); err != nil {
		return nil, err
	}
	// Nested documents decode as bson.D: they keep the codec's key order (and
	// any repeated key of an encoded bson.D), as the driver would store them.
	var out bson.M
	if err := bson.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("behavior/mongo: decode insert document: %w", err)
	}
	return out, nil
}
