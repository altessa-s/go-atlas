// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"net/url"
	"reflect"
	"slices"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	coremaps "github.com/altessa-s/go-atlas-core/collections/maps"
)

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
	if registryLeaf(ec.reg, t) {
		return true
	}
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
		z, _ := reflect.TypeAssert[bson.Zeroer](fv)
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
		// A registry leaf is returned as is too: the driver encodes it with
		// the same registry.
		return fv.Interface(), nil
	}
	raw, err := encodeWrapped(fv, ec.minSize, fv.CanAddr() && !ec.noAddr, ec.reg)
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
// it holds a copy, as non-addressable as fv is to the codec. A non-nil reg
// encodes in place of the default registry.
func encodeWrapped(fv reflect.Value, minSize, addr bool, reg *bson.Registry) (bson.RawValue, error) {
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
	doc, err := marshal(wrapper.Interface(), reg)
	if err != nil {
		return bson.RawValue{}, err
	}
	return bson.Raw(doc).LookupErr("v")
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
