// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/domain/behavior"

	coremaps "github.com/altessa-s/go-atlas/core/collections/maps"
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
}

// foldObject builds the set/unset maps for a single resolved object, laid out
// as the BSON struct codec lays out the object's struct type (see
// [layoutOf]): inline struct fields sit at the parent level, an embedded
// struct without inline is a subdocument, and an inline map's entries join
// the parent. On update, fields the engine marked Strip (via
// behavior.WithKinds on the engine) are excluded from both operators. It
// applies at every nesting level: nested objects (struct pointers, slice
// elements, map values) all come from the engine's walk and carry their own
// Strip marks.
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
		if m == modeUpdate && df.omitOnUpdate {
			continue
		}
		f, exact, ok := resolveField(obj, df)
		switch {
		case ok && !exact:
			// A stripped inline ancestor governs the field. Update drops it;
			// insert is unfiltered and writes the stored value as is.
			if m == modeUpdate {
				continue
			}
			fv, ferr := obj.Value.FieldByIndexErr(df.index)
			if ferr != nil {
				continue
			}
			f = &behavior.Field{Index: df.index, Name: df.goName, Tag: df.tag, Value: fv}
		case !ok:
			// Behind a nil pointer: the codec writes no key.
			continue
		}
		if m == modeUpdate && f.Strip {
			continue
		}
		fieldCtx := ec
		if crossesPointer(obj.Value.Type(), df.index) {
			// Reached through an inline pointer: addressable to the codec.
			fieldCtx.noAddr = false
		}
		if err := foldField(f, df, o, m, fieldCtx, set, unset); err != nil {
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

// foldInlineMap merges the entries of the object's inline map into its parent
// document. A key that names any field of the layout is an error, as in the
// codec, whether or not that field holds a value. A nil or empty map, or one
// stripped on update, contributes nothing: its keys are dynamic, so there is
// no stored name to $unset.
func foldInlineMap(obj behavior.Object, layout *docLayout, o *options, m mode, ec encCtx, set bson.M) error {
	loc := locate(obj, []int{layout.inlineMap})
	f, exact, ok := loc.field, loc.exact, loc.field != nil
	if !ok || !exact || (m == modeUpdate && f.Strip) || f.Value.Len() == 0 {
		return nil
	}
	if m == modeUpdate && hasOmitOnUpdate(o.fieldTag(f.Tag)) {
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
	// pointer and an empty collection.
	if m == modeInsert && omitEmpty && isCodecEmpty(fv) {
		return nil
	}
	if m == modeUpdate && ((fv.Kind() == reflect.Pointer && fv.IsNil()) || isEmptyCollection(fv)) {
		unset[name] = nil
		return nil
	}

	v, err := foldValue(fv, f.Nested, f.Collection, o, m, ec)
	if err != nil {
		return fmt.Errorf("behavior/mongo: %s: %w", name, err)
	}
	set[name] = v
	return nil
}

// crossesPointer reports whether the Go index path index, taken from struct
// type t, passes through a pointer before its last field.
func crossesPointer(t reflect.Type, index []int) bool {
	for _, i := range index[:len(index)-1] {
		t = t.Field(i).Type
		if t.Kind() == reflect.Pointer {
			return true
		}
	}
	return false
}

// isCodecLeaf reports whether the codec may encode fv as a unit that folding
// must not take apart: a type with a BSON marshaler on the value or the
// pointer receiver (encodeLeaf lets the codec decide which applies), a byte
// slice or array, which the codec writes as binary, or a slice convertible to
// bson.D, which it writes as a document rather than an array.
func isCodecLeaf(fv reflect.Value, ec encCtx) bool {
	t := fv.Type()
	if pointerOnlyMarshaler(t) {
		// The codec uses the marshaler only for an addressable value; for a
		// copy — a map value — it falls back to the type's ordinary encoding:
		// its built-in rules still apply, and folding reproduces the rest
		// with strip marks applied.
		return (fv.CanAddr() && !ec.noAddr) || isBuiltinLeafType(t)
	}
	return isCodecLeafType(t)
}

// pointerOnlyMarshaler reports whether t's BSON marshaler is declared on the
// pointer receiver only.
func pointerOnlyMarshaler(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer || t.Kind() == reflect.Interface {
		return false
	}
	if t.Implements(bsonMarshalerType) || t.Implements(bsonValueMarshalerType) {
		return false
	}
	return implementsBSONMarshaler(t)
}

// isCodecLeafType is isCodecLeaf for a type. Besides marshalers it covers
// the structs the codec's default registry has dedicated encoders for (see
// codecStructTypes) and arrays of bson.E, which it writes as documents.
func isCodecLeafType(t reflect.Type) bool {
	return implementsBSONMarshaler(t) || isBuiltinLeafType(t)
}

// isBuiltinLeafType reports whether the codec encodes t as a unit by its own
// rules, marshalers aside: a struct with a dedicated type encoder (see
// codecStructTypes), a byte slice or array (binary), a slice convertible to
// bson.D or an array of bson.E (a document).
func isBuiltinLeafType(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Struct:
		return codecStructTypes.Contains(t)
	case reflect.Slice:
		return t.Elem().Kind() == reflect.Uint8 || t.ConvertibleTo(bsonDType)
	case reflect.Array:
		return t.Elem().Kind() == reflect.Uint8 || t.Elem() == bsonEType
	default:
		return false
	}
}

// Types with dedicated codec encoders.
var (
	// bsonDType is bson.D, which the codec's slice encoder writes as a
	// document.
	bsonDType = reflect.TypeFor[bson.D]()
	// bsonEType is bson.E; the codec writes an array of it as a document.
	bsonEType = reflect.TypeFor[bson.E]()
)

// codecStructTypes are the struct types the v2 codec's default registry
// encodes with dedicated type encoders (default_value_encoders.go,
// primitive_codecs.go) rather than field by field. bson.E is not among them: on its own it is an ordinary
// struct.
var codecStructTypes = coremaps.NewImmutableMap(map[reflect.Type]struct{}{
	reflect.TypeFor[time.Time]():          {},
	reflect.TypeFor[url.URL]():            {},
	reflect.TypeFor[bson.Binary]():        {},
	reflect.TypeFor[bson.Vector]():        {},
	reflect.TypeFor[bson.Undefined]():     {},
	reflect.TypeFor[bson.Null]():          {},
	reflect.TypeFor[bson.Regex]():         {},
	reflect.TypeFor[bson.DBPointer]():     {},
	reflect.TypeFor[bson.Timestamp]():     {},
	reflect.TypeFor[bson.MinKey]():        {},
	reflect.TypeFor[bson.MaxKey]():        {},
	reflect.TypeFor[bson.CodeWithScope](): {},
	reflect.TypeFor[bson.Decimal128]():    {},
	reflect.TypeFor[bson.RawValue]():      {}, // primitive_codecs.go
})

// isCodecEmpty reports whether the codec's omitempty drops fv (v2
// struct_codec.go isEmpty, without omitzerostruct): a nil interface; any
// value implementing bson.Zeroer whose IsZero reports true; an empty array,
// map, slice or string; a nil pointer or other zero scalar. A struct is never
// empty unless it is a Zeroer.
func isCodecEmpty(fv reflect.Value) bool {
	if !fv.IsValid() {
		return true
	}
	kind := fv.Kind()
	if kind == reflect.Interface {
		if fv.IsNil() {
			return true
		}
		// The codec has no encoder for an interface type other than any, so
		// it unwraps the dynamic value before testing it.
		if fv.Type().NumMethod() == 0 {
			return false
		}
		return isCodecEmpty(fv.Elem())
	}
	if (kind != reflect.Pointer || !fv.IsNil()) && fv.Type().Implements(zeroerType) {
		z, _ := fv.Interface().(bson.Zeroer)
		return z.IsZero()
	}
	switch kind {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return fv.Len() == 0
	case reflect.Struct:
		return false
	default:
		return fv.IsZero()
	}
}

// zeroerType is bson.Zeroer, consulted by the codec's omitempty.
var zeroerType = reflect.TypeFor[bson.Zeroer]()

// encodeLeaf returns the value to store for fv, a value the translator does
// not fold, so that the codec writes it exactly as it would in fv's place.
// Under minsize the codec's rules depend on each value's registered encoder
// and reach through pointers, interfaces, collections and nested structs; a
// BSON marshaler anywhere in the value may depend on addressability (a
// pointer-receiver one) or fail. In those cases the codec itself encodes fv
// (see [encodeWrapped]) and an encoding error is returned rather than
// deferred: an integer comes back as int32 or unchanged, anything else as the
// encoded bson.RawValue, which the codec writes verbatim. Otherwise fv is
// returned as is.
func encodeLeaf(fv reflect.Value, ec encCtx) (any, error) {
	if !fv.IsValid() {
		return nil, nil //nolint:nilnil // a nil interface element is stored as null
	}
	if !ec.minSize && !needsCodec(dynamicType(fv)) {
		return fv.Interface(), nil
	}
	raw, err := encodeWrapped(fv, ec.minSize, fv.CanAddr() && !ec.noAddr)
	if err != nil {
		return nil, err
	}
	if !isPlainInteger(fv.Type()) {
		return raw, nil
	}
	if raw.Type == bson.TypeInt32 {
		return raw.Int32(), nil
	}
	return fv.Interface(), nil
}

// dynamicType returns the type the codec encodes fv as: the dynamic type of a
// non-nil interface, otherwise fv's own type.
func dynamicType(fv reflect.Value) reflect.Type {
	if fv.Kind() == reflect.Interface && !fv.IsNil() {
		return fv.Elem().Type()
	}
	return fv.Type()
}

// needsCodecCache memoizes needsCodec per type.
var needsCodecCache sync.Map // map[reflect.Type]bool

// needsCodec reports whether a value of type t has to be encoded by the codec
// to come out right: t, or a type the codec reaches through it (struct fields,
// pointer targets, slice, array and map elements), has a BSON marshaler on
// the value or the pointer receiver, or is an interface whose dynamic value
// might.
func needsCodec(t reflect.Type) bool {
	if v, ok := needsCodecCache.Load(t); ok {
		b, _ := v.(bool)
		return b
	}
	b := needsCodecWalk(t, nil)
	needsCodecCache.Store(t, b)
	return b
}

// needsCodecWalk is needsCodec with a guard against recursive types.
func needsCodecWalk(t reflect.Type, seen []reflect.Type) bool {
	if slices.Contains(seen, t) {
		return false
	}
	seen = append(seen, t)
	if implementsBSONMarshaler(t) {
		return true
	}
	switch t.Kind() {
	case reflect.Interface:
		return true
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return needsCodecWalk(t.Elem(), seen)
	case reflect.Struct:
		for sf := range t.Fields() {
			if sf.IsExported() && needsCodecWalk(sf.Type, seen) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// isPlainInteger reports whether t is an integer type the codec encodes with
// its integer encoders, without a BSON marshaler of its own.
func isPlainInteger(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return !implementsBSONMarshaler(t)
	default:
		return false
	}
}

// wrapperKey identifies a cached one-field wrapper type.
type wrapperKey struct {
	t       reflect.Type
	minSize bool
}

// wrappers caches, per field type and minsize flag, a one-field struct type
// whose field the codec encodes on request.
var wrappers sync.Map // map[wrapperKey]reflect.Type

// encodeWrapped encodes fv as the codec encodes a field of its type, tagged
// minsize when minSize is set. When addr is set, the wrapper holds a pointer
// to fv itself: the codec reaches fv through it with its addressability and
// its receiver identity — a pointer marshaler sees fv's own address. Otherwise
// it holds a copy, as non-addressable as fv is to the codec.
func encodeWrapped(fv reflect.Value, minSize, addr bool) (bson.RawValue, error) {
	ft := fv.Type()
	if addr {
		ft = reflect.PointerTo(ft)
	}
	key := wrapperKey{t: ft, minSize: minSize}
	cached, ok := wrappers.Load(key)
	if !ok {
		tag := `bson:"v"`
		if minSize {
			tag = `bson:"v,minsize"`
		}
		cached, _ = wrappers.LoadOrStore(key, reflect.StructOf([]reflect.StructField{{
			Name: "V", Type: ft, Tag: reflect.StructTag(tag),
		}}))
	}
	wt, _ := cached.(reflect.Type)
	wrapper := reflect.New(wt).Elem()
	if addr {
		wrapper.Field(0).Set(fv.Addr())
	} else {
		wrapper.Field(0).Set(fv)
	}
	doc, err := bson.Marshal(wrapper.Interface())
	if err != nil {
		return bson.RawValue{}, err
	}
	return bson.Raw(doc).LookupErr("v")
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

// isEmptyCollection reports whether fv is an empty slice, map, or array.
func isEmptyCollection(fv reflect.Value) bool {
	switch fv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array:
		return fv.Len() == 0
	default:
		return false
	}
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
	for field := range t.Fields() {
		if field.IsExported() {
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
	minSize      bool
	inline       bool
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
	info := bsonTag(tag, goName)
	return info.name, info.omitEmpty, info.omitOnUpdate
}

// fieldTag returns the tag string the codec reads for a field: the value of
// the configured tag key, or — for the default bson key, when it is absent —
// the whole struct tag if it holds no key:value pair (the codec's legacy
// bare-tag form, `Secret string "private"`).
func (o *options) fieldTag(st reflect.StructTag) string {
	tag, ok := st.Lookup(o.bsonTagName)
	if !ok && o.bsonTagName == DefaultBSONTagName && len(st) > 0 && !strings.Contains(string(st), ":") {
		return string(st)
	}
	return tag
}

// bsonTag parses and memoizes a bson tag; see [parseBSONTag].
func bsonTag(tag, goName string) bsonTagInfo {
	key := bsonTagKey{tag: tag, goName: goName}
	if v, ok := bsonTagCache.Load(key); ok {
		info, _ := v.(bsonTagInfo)
		return info
	}

	// As in the codec (struct_tag_parser.go parseTags): only the complete
	// tag "-" skips the field; options are recognized in every token, the
	// first one included, and a non-empty first token names the field.
	var info bsonTagInfo
	if tag != "-" {
		info.name = strings.ToLower(goName)
		for idx, tok := range strings.Split(tag, ",") {
			if idx == 0 && tok != "" {
				info.name = tok
			}
			switch tok {
			case "omitempty":
				info.omitEmpty = true
			case "omitonupdate":
				info.omitOnUpdate = true
			case "minsize":
				info.minSize = true
			case "inline":
				info.inline = true
			}
		}
	}

	bsonTagCache.Store(key, info)
	return info
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
// ({"$set": …, "$unset": …}) under opts. "_id" is removed from both; nil
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
		set, unset, err := foldObject(root, t.o, modeUpdate, encCtx{})
		if err != nil {
			return nil, err
		}
		// _id is immutable in MongoDB: neither operator may touch it, whether
		// it is an own field or one an inline struct contributes.
		delete(set, "_id")
		delete(unset, "_id")
		// Top-level operator keys are paths: a field whose stored name holds a
		// dot (bson:"a.b", a literal key to the codec) would address another
		// field, and a "$" prefix is not a field at all.
		for _, op := range []bson.M{set, unset} {
			for k := range op {
				if err := checkPathName(k); err != nil {
					return nil, err
				}
			}
		}
		result := bson.M{"$set": set}
		if len(unset) > 0 {
			result["$unset"] = unset
		}
		return result, nil
	}

	if t.o.bsonTagName == DefaultBSONTagName && root.Value.IsValid() {
		return encodeInsert(root.Value)
	}
	set, _, err := foldObject(root, t.o, modeInsert, encCtx{})
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

// checkPathName returns [ErrUnaddressableName] for a name that cannot be one
// segment of a document path.
func checkPathName(name string) error {
	if strings.Contains(name, ".") || strings.HasPrefix(name, "$") {
		return fmt.Errorf("%w: %q", ErrUnaddressableName, name)
	}
	return nil
}

// encodeInsert builds an insert document by encoding v with the BSON codec
// itself — an insert applies no behavior filtering, so the document is
// exactly what the driver would store for v, inline fields, omitempty,
// minsize and marshalers included — and decoding it into a bson.M (nested
// documents as ordered bson.D, arrays as bson.A, values as their BSON Go
// types). v is
// encoded through its address when it has one, as the driver encodes a
// pointer. Keys starting with "$" are rejected at any depth, as on the folding
// path, so no map can smuggle an operator-like key into the document.
func encodeInsert(v reflect.Value) (bson.M, error) {
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
	raw, err := bson.Marshal(target)
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

// maxValidateDepth bounds validateValue, so a cyclic pointer graph — which
// the codec would also never finish encoding — fails instead of recursing.
const maxValidateDepth = 256

// validateValue walks v along the paths the codec encodes — the fields of
// each struct's layout, non-nil pointer targets, dynamic interface values,
// slice, array and map elements — and runs layoutOf on every struct type it
// meets, so a declaration the codec cannot describe (it recurses forever on a
// struct inlined into itself) fails here with an error. Fields tagged "-",
// nil pointers and types with their own BSON marshaler are not described by
// the codec and are skipped.
func validateValue(v reflect.Value, depth int) error {
	if depth > maxValidateDepth {
		return fmt.Errorf("behavior/mongo: value nests deeper than %d levels", maxValidateDepth)
	}
	if !v.IsValid() || selectsMarshaler(v) {
		return nil
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return validateValue(v.Elem(), depth+1)
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return nil
		}
		for i := range v.Len() {
			if err := validateValue(v.Index(i), depth+1); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		for it := v.MapRange(); it.Next(); {
			if err := validateValue(it.Value(), depth+1); err != nil {
				return err
			}
		}
		return nil
	case reflect.Struct:
		layout, err := layoutOf(v.Type(), DefaultBSONTagName)
		if err != nil {
			return err
		}
		for _, df := range layout.fields {
			fv, err := v.FieldByIndexErr(df.index)
			if err != nil {
				continue // behind a nil inline pointer: not encoded
			}
			if df.omitEmpty && isCodecEmpty(fv) {
				continue // omitted: not encoded
			}
			if err := validateValue(fv, depth+1); err != nil {
				return err
			}
		}
		if layout.inlineMap >= 0 {
			return validateValue(v.Field(layout.inlineMap), depth+1)
		}
		return nil
	default:
		return nil
	}
}

// staticSafeCache memoizes staticallySafe per type.
var staticSafeCache sync.Map // map[reflect.Type]bool

// staticallySafe reports whether every struct type a value of type t can
// reach — through layout fields, pointers, slices, arrays and maps — has a
// valid layout and no interface sits on the way, so no value of t can lead
// the codec into a declaration it cannot describe. Such types skip the
// per-value [validateValue] walk; any other type gets it, which also accepts
// the invalid types a given value never encodes (nil, omitted or ignored).
func staticallySafe(t reflect.Type) bool {
	if v, ok := staticSafeCache.Load(t); ok {
		b, _ := v.(bool)
		return b
	}
	b := staticWalk(t, map[reflect.Type]walkState{})
	staticSafeCache.Store(t, b)
	return b
}

// walkState tracks a type during staticWalk.
type walkState uint8

const (
	walkActive walkState = iota + 1 // on the current path
	walkDone                        // finished and safe
)

// staticWalk is staticallySafe with cycle detection. A type met again while
// still on the current path makes the graph recursive: its values can nest
// without bound or form a cycle, so they keep the per-value walk, which
// bounds the depth. A type already finished is simply safe.
func staticWalk(t reflect.Type, seen map[reflect.Type]walkState) bool {
	switch seen[t] {
	case walkActive:
		return false
	case walkDone:
		return true
	}
	seen[t] = walkActive
	if !staticKind(t, seen) {
		return false
	}
	seen[t] = walkDone
	return true
}

// staticKind checks one type's layers for staticWalk.
func staticKind(t reflect.Type, seen map[reflect.Type]walkState) bool {
	switch t.Kind() {
	case reflect.Interface:
		return false
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return staticWalk(t.Elem(), seen)
	case reflect.Struct:
		if implementsBSONMarshaler(t) {
			// Encoded by its marshaler only when addressable; stay
			// conservative and let the value walk decide.
			return false
		}
		layout, err := layoutOf(t, DefaultBSONTagName)
		if err != nil {
			return false
		}
		for _, df := range layout.fields {
			if !staticWalk(t.FieldByIndex(df.index).Type, seen) {
				return false
			}
		}
		if layout.inlineMap >= 0 {
			return staticWalk(t.Field(layout.inlineMap).Type, seen)
		}
		return true
	default:
		return true
	}
}

// selectsMarshaler reports whether the codec encodes v with a BSON marshaler
// rather than describing its type: the type implements one itself, or v is
// addressable and its pointer does.
func selectsMarshaler(v reflect.Value) bool {
	t := v.Type()
	if t.Implements(bsonMarshalerType) || t.Implements(bsonValueMarshalerType) {
		return true
	}
	return v.CanAddr() && implementsBSONMarshaler(t)
}

// rejectOperatorKeys fails on a key starting with "$" in doc or any document
// nested in it, including documents inside arrays and the scope of JavaScript
// code with scope.
func rejectOperatorKeys(doc bson.Raw) error {
	elems, err := doc.Elements()
	if err != nil {
		return fmt.Errorf("behavior/mongo: insert document: %w", err)
	}
	for _, e := range elems {
		if key := e.Key(); strings.HasPrefix(key, "$") {
			return fmt.Errorf("behavior/mongo: key %q must not start with %q (reserved for MongoDB operators)", key, "$")
		}
		val := e.Value()
		switch val.Type {
		case bson.TypeEmbeddedDocument:
			if err := rejectOperatorKeys(val.Document()); err != nil {
				return err
			}
		case bson.TypeArray:
			if err := rejectOperatorKeys(bson.Raw(val.Array())); err != nil {
				return err
			}
		case bson.TypeCodeWithScope:
			// The scope of JavaScript code with scope is a document too.
			if _, scope, ok := val.CodeWithScopeOK(); ok {
				if err := rejectOperatorKeys(scope); err != nil {
					return err
				}
			}
		default:
		}
	}
	return nil
}
