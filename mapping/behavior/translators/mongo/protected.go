// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"reflect"
	"slices"
	"strings"
	"sync"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/mapping/behavior"
)

// holdsProtected reports whether replacing the subdocument folded from obj
// could delete a stored value the update must keep. It follows the BSON
// layout of obj's struct (see [layoutOf]), as the fold does: a key tagged — or
// imported through an inline struct tagged — omitonupdate; a key behind an
// unset inline pointer, which the replacement would remove although the
// update says nothing about it; a key governed by a stripped inline ancestor
// or by an inline struct tagged identifier; a stripped or identifier field; a
// stripped, identifier or omitonupdate inline map; and the same, recursively,
// in a nested struct written as a folded subdocument. Fields BSON ignores
// (bson:"-") are not in the layout and protect nothing. Collections are not
// descended: their elements are checked when folded.
func (o *options) holdsProtected(obj behavior.Object, ec encCtx) bool {
	layout, err := o.objectLayout(obj)
	if err != nil {
		// The fold reports the error.
		return false
	}
	for _, df := range layout.fields {
		if df.omitOnUpdate {
			return true
		}
		f, exact, ok := resolveField(obj, df)
		if !ok || !exact {
			// Behind a nil inline pointer, or under a stripped inline
			// ancestor.
			return true
		}
		if f.Strip || isIdentifier(f) || identifierAncestor(obj, df.index) {
			return true
		}
		fieldCtx := ec
		if crossesPointer(obj.Value.Type(), df.index) {
			fieldCtx.noAddr = false
		}
		if sub, subCtx := pathObject(f.Value, f.Nested, fieldCtx); sub != nil && o.holdsProtected(*sub, subCtx) {
			return true
		}
	}
	if layout.inlineMap >= 0 {
		loc := locate(obj, []int{layout.inlineMap})
		if f := loc.field; f != nil && (f.Strip || isIdentifier(f) || o.omitOnUpdate(f.Tag)) {
			return true
		}
	}
	return false
}

// protectedKey identifies a memoized [options.protectedType] result.
type protectedKey struct {
	t      reflect.Type
	inColl bool
	noAddr bool
	pol    behavior.Policy
	tag    string
	reg    *bson.Registry
}

// protectedCache memoizes protectedType. Declared types and policies are
// finite, so an unbounded sync.Map suffices.
var protectedCache sync.Map // map[protectedKey]bool

// protectedVisit is a struct type on the current protectedWalk path.
type protectedVisit struct {
	t      reflect.Type
	inColl bool
	noAddr bool
}

// protectedType reports whether a value of declared type t can hold a field
// the update must keep, judged from the type alone: an update that removes such
// a value — a nil pointer, a nil or empty collection, a nil element — has no
// resolved fields whose strip marks could be checked, yet deletes whatever is
// stored below it. The judgment follows the engine's policy pol (see
// [behavior.PolicyBinder]), the one behind the strip marks of resolved values,
// and the BSON layout of every struct reached through pointers and collections
// (see [layoutOf]). A field is protected when it, or an inline struct it is
// imported through, carries a kind pol strips (read under pol's tag key);
// outside a collection, an identifier or an omitonupdate field is protected
// too, whatever pol strips, as the update never writes either. Inside a
// collection (inColl, and below any collection layer of t) an identifier and
// an omitonupdate field belong to the element and are replaced with it, and a
// field stripped only for being an identifier is exempt when its value is a
// unit, as in [excludeOnUpdate]. A type the codec encodes as a unit holds
// nothing to protect; as in isCodecLeaf this depends on addressability for a
// pointer-receiver marshaler, which the codec uses only for a value it can
// address: noAddr marks a value reached without addressability (a map value,
// or anything below one until a pointer or slice is crossed), which the codec
// encodes field by field.
func (o *options) protectedType(t reflect.Type, inColl, noAddr bool, pol behavior.Policy) bool {
	key := protectedKey{t: t, inColl: inColl, noAddr: noAddr, pol: pol, tag: o.bsonTagName, reg: o.registry}
	if v, ok := protectedCache.Load(key); ok {
		b, _ := v.(bool)
		return b
	}
	b := o.protectedWalk(t, inColl, noAddr, pol, nil)
	protectedCache.Store(key, b)
	return b
}

// protectedWalk is protectedType without the cache. seen holds the struct
// types on the current path; a recursive type is not descended twice.
func (o *options) protectedWalk(t reflect.Type, inColl, noAddr bool, pol behavior.Policy, seen []protectedVisit) bool {
	for range maxCollectionLayers {
		if o.leafType(t, noAddr) {
			return false
		}
		// Addressability follows the codec (see foldValue): a pointer's
		// target and a slice's elements are addressable, a map's values are
		// not, an array's elements are when the array is.
		switch t.Kind() {
		case reflect.Pointer:
			t = t.Elem()
			noAddr = false
		case reflect.Slice:
			t = t.Elem()
			inColl, noAddr = true, false
		case reflect.Array:
			t = t.Elem()
			inColl = true
		case reflect.Map:
			t = t.Elem()
			inColl, noAddr = true, true
		case reflect.Struct:
			return o.protectedStruct(t, inColl, noAddr, pol, seen)
		default:
			return false
		}
	}
	return false
}

// protectedStruct is protectedWalk for struct type t.
func (o *options) protectedStruct(t reflect.Type, inColl, noAddr bool, pol behavior.Policy, seen []protectedVisit) bool {
	visit := protectedVisit{t: t, inColl: inColl, noAddr: noAddr}
	if slices.Contains(seen, visit) {
		return false
	}
	seen = append(seen, visit)
	layout, err := layoutOf(t, o.bsonTagName)
	if err != nil {
		// The fold reports the error.
		return false
	}
	for _, df := range layout.fields {
		if o.protectedField(t, df, inColl, noAddr, pol, seen) {
			return true
		}
	}
	if layout.inlineMap < 0 {
		return false
	}
	sf := t.Field(layout.inlineMap)
	// A map is compound: no element-identifier exemption.
	if o.protectedKinds(sf, inColl, true, pol) || (!inColl && o.omitOnUpdate(sf.Tag)) {
		return true
	}
	return o.protectedWalk(sf.Type, inColl, noAddr, pol, seen)
}

// protectedField reports whether the layout field df of struct type t is
// protected, or can hold a protected field, under protectedType's rules.
func (o *options) protectedField(t reflect.Type, df docField, inColl, noAddr bool, pol behavior.Policy, seen []protectedVisit) bool {
	if !inColl && df.omitOnUpdate {
		return true
	}
	// Reached through an inline pointer: addressable to the codec.
	fieldNoAddr := noAddr && !crossesPointer(t, df.index)
	// Every inline struct the field is imported through (compound), then
	// the field itself.
	for i := range df.index {
		sf := t.FieldByIndex(df.index[:i+1])
		compound := i < len(df.index)-1 || o.compoundType(sf.Type, fieldNoAddr)
		if o.protectedKinds(sf, inColl, compound, pol) {
			return true
		}
	}
	return o.protectedWalk(t.FieldByIndex(df.index).Type, inColl, fieldNoAddr, pol, seen)
}

// protectedKinds reports whether the behavior kinds of field sf, read under
// pol's tag key, make it protected: outside a collection a kind pol strips or
// identifier; inside one a kind pol strips, unless identifier is its only kind
// and its value is not compound (the element-identifier exemption of
// [excludeOnUpdate]). Unknown tokens are ignored; the engine rejects them.
func (o *options) protectedKinds(sf reflect.StructField, inColl, compound bool, pol behavior.Policy) bool {
	stripped, ident, onlyIdent := false, false, true
	for tok := range strings.SplitSeq(sf.Tag.Get(pol.TagName()), ",") {
		k, ok := behavior.ParseKind(strings.TrimSpace(tok))
		if !ok {
			continue
		}
		stripped = stripped || pol.Strips(k)
		ident = ident || k == behavior.Identifier
		onlyIdent = onlyIdent && k == behavior.Identifier
	}
	if !inColl {
		return stripped || ident
	}
	exempt := ident && onlyIdent && !compound
	return stripped && !exempt
}

// compoundType is isCompound for a declared type, reached without
// addressability when noAddr is set: through pointers, a struct folded field
// by field, or a slice, array or map that is not a codec leaf.
func (o *options) compoundType(t reflect.Type, noAddr bool) bool {
	for range maxCollectionLayers {
		if o.leafType(t, noAddr) {
			return false
		}
		switch t.Kind() {
		case reflect.Pointer:
			t = t.Elem()
			noAddr = false
		case reflect.Struct:
			return recurseIntoStruct(t) || (pointerOnlyMarshaler(t) && hasExportedField(t))
		case reflect.Slice, reflect.Array, reflect.Map:
			return true
		default:
			return false
		}
	}
	return false
}

// leafType is isCodecLeaf for a declared type: a registry leaf, a type with a
// BSON marshaler — a pointer-receiver one only when the value is addressable
// (not noAddr) — or a type with a built-in encoding of its own.
func (o *options) leafType(t reflect.Type, noAddr bool) bool {
	if registryLeaf(o.registry, t) {
		return true
	}
	if pointerOnlyMarshaler(t) {
		return !noAddr || isBuiltinLeafType(t)
	}
	return isCodecLeafType(t)
}
