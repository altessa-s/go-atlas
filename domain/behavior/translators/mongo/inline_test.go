// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"bytes"
	"fmt"
	"net/url"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"

	"github.com/altessa-s/go-atlas/domain/behavior"

	mongo "github.com/altessa-s/go-atlas/domain/behavior/translators/mongo"
)

type inlineMeta struct {
	Version int    `bson:"version"`
	Owner   string `bson:"owner" behavior:"immutable"`
	Name    string `bson:"name"`
}

type inlineDoc struct {
	ID     string         `bson:"_id"`
	Name   string         `bson:"name"`
	Meta   inlineMeta     `bson:",inline"`
	Extras map[string]any `bson:",inline"`
}

// decodeM normalizes a document through the BSON codec so a translator output
// and a driver encoding compare equal regardless of map order and int width.
func decodeM(t *testing.T, raw []byte) bson.M {
	t.Helper()
	dec := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(raw)))
	dec.DefaultDocumentM()
	var out bson.M
	require.NoError(t, dec.Decode(&out))
	return out
}

func TestInsertInlineMatchesDriver(t *testing.T) {
	t.Parallel()

	type ptrOnly struct {
		Name string      `bson:"name"`
		Ptr  *inlineMeta `bson:",inline"`
	}

	tests := []struct {
		name string
		doc  any
	}{
		{
			name: "inline struct and map, parent field shadows",
			doc: &inlineDoc{
				ID: "1", Name: "parent",
				Meta:   inlineMeta{Version: 3, Owner: "o", Name: "shadowed"},
				Extras: map[string]any{"color": "red"},
			},
		},
		{name: "nil inline pointer writes nothing", doc: &ptrOnly{Name: "n"}},
		{name: "inline pointer", doc: &ptrOnly{Name: "n", Ptr: &inlineMeta{Version: 1, Owner: "o", Name: "x"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			want, err := bson.Marshal(tc.doc)
			require.NoError(t, err)
			gotDoc := insert(t, tc.doc)
			got, err := bson.Marshal(gotDoc)
			require.NoError(t, err)
			require.Equal(t, decodeM(t, want), decodeM(t, got))
		})
	}
}

func TestInsertInlineMapConflict(t *testing.T) {
	t.Parallel()

	doc := &inlineDoc{Name: "n", Extras: map[string]any{"name": "clash"}}
	_, driverErr := bson.Marshal(doc)
	require.Error(t, driverErr, "the codec rejects the clash too")

	eng := behavior.New[bson.M](mongo.NewInsertTranslator())
	_, err := eng.Translate(t.Context(), doc)
	require.Error(t, err)
}

func TestInsertInlineMapRejectsOperatorKey(t *testing.T) {
	t.Parallel()

	eng := behavior.New[bson.M](mongo.NewInsertTranslator())
	_, err := eng.Translate(t.Context(), &inlineDoc{Extras: map[string]any{"$set": 1}})
	require.Error(t, err)
}

func TestUpdateInline(t *testing.T) {
	t.Parallel()

	got := update(t, &inlineDoc{
		ID: "1", Name: "parent",
		Meta:   inlineMeta{Version: 2, Owner: "o", Name: "shadowed"},
		Extras: map[string]any{"color": "red"},
	}, []behavior.Option{behavior.WithKinds(behavior.DefaultUpdateKinds...)})

	require.Equal(t, bson.M{
		"$set": bson.M{"name": "parent", "version": 2, "color": "red"},
	}, got, "inlined fields land at the parent level; the immutable owner is stripped; nil inline pointer is not unset")
}

func TestUpdateInlineUnsetsNestedNil(t *testing.T) {
	t.Parallel()

	type opt struct {
		Note *string `bson:"note"`
	}
	type doc struct {
		Name string `bson:"name"`
		Opt  opt    `bson:",inline"`
	}
	got := update(t, &doc{Name: "n"}, nil)
	require.Equal(t, bson.M{
		"$set":   bson.M{"name": "n"},
		"$unset": bson.M{"note": nil},
	}, got, "a nil pointer inside an inline struct unsets its parent-level key")
}

// matchesDriver asserts that the insert translator writes what the codec
// writes, or fails where the codec fails.
func matchesDriver(t *testing.T, doc any) {
	t.Helper()

	want, driverErr := bson.Marshal(doc)
	eng := behavior.New[bson.M](mongo.NewInsertTranslator())
	got, err := eng.Translate(t.Context(), doc)
	if driverErr != nil {
		require.Error(t, err, "the codec rejects this document: %v", driverErr)
		return
	}
	require.NoError(t, err)
	raw, err := bson.Marshal(got)
	require.NoError(t, err)
	require.Equal(t, decodeM(t, want), decodeM(t, raw))
}

type deepMeta struct {
	Name string `bson:"name"`
	Deep string `bson:"deep"`
}

type midMeta struct {
	Deep  string   `bson:"deep"`
	Inner deepMeta `bson:",inline"`
}

func TestInsertInlineCodecParity(t *testing.T) {
	t.Parallel()

	type omitted struct {
		Name  string     `bson:"name,omitempty"`
		Inner inlineMeta `bson:",inline"`
	}
	type depth struct {
		Mid midMeta `bson:",inline"`
	}
	type nilDup struct {
		A inlineMeta  `bson:",inline"`
		B *inlineMeta `bson:",inline"`
	}
	type mapVsOmitted struct {
		Name string         `bson:"name,omitempty"`
		M    map[string]any `bson:",inline"`
	}
	type mapVsNilInline struct {
		P *inlineMeta    `bson:",inline"`
		M map[string]any `bson:",inline"`
	}
	type mapBeforeStruct struct {
		M     map[string]any `bson:",inline"`
		Inner inlineMeta     `bson:",inline"`
	}
	type nestedMap struct {
		Inner struct {
			X int            `bson:"x"`
			M map[string]any `bson:",inline"`
		} `bson:",inline"`
	}
	type nilIntPtr struct {
		P *int `bson:",inline"`
	}
	type intKeyMap struct {
		M map[int]string `bson:",inline"`
	}
	type myKey string
	type definedKeyMap struct {
		M map[myKey]string `bson:",inline"`
	}
	type twoMaps struct {
		A map[string]any `bson:",inline"`
		B map[string]any `bson:",inline"`
	}
	type embedded struct {
		Credentials
		Name string `bson:"name"`
	}
	type embeddedPtr struct {
		*Credentials
	}

	tests := []struct {
		name string
		doc  any
	}{
		{name: "own omitempty field still shadows the inline one", doc: &omitted{Inner: inlineMeta{Name: "inner", Version: 1}}},
		{name: "shallower inline field dominates a deeper one", doc: &depth{Mid: midMeta{Deep: "mid", Inner: deepMeta{Name: "n", Deep: "deep"}}}},
		{name: "duplicate behind a nil pointer is still a duplicate", doc: &nilDup{}},
		{name: "map key against an omitted field", doc: &mapVsOmitted{M: map[string]any{"name": 1}}},
		{name: "map key against a nil inline pointer field", doc: &mapVsNilInline{M: map[string]any{"version": 1}}},
		{name: "map declared before an inline struct", doc: &mapBeforeStruct{M: map[string]any{"version": 1}}},
		{name: "map inside an inline struct is not written", doc: &nestedMap{}},
		{name: "nil inline pointer to non-struct", doc: &nilIntPtr{}},
		{name: "inline map with int keys", doc: &intKeyMap{}},
		{name: "inline map with a defined string key type", doc: &definedKeyMap{M: map[myKey]string{"a": "b"}}},
		{name: "two inline maps", doc: &twoMaps{A: map[string]any{"a": 1}, B: map[string]any{"b": 2}}},
		{name: "embedded struct without inline is a subdocument", doc: &embedded{Credentials: Credentials{Secret: "s", Login: "l"}, Name: "n"}},
		{name: "nil embedded pointer without inline", doc: &embeddedPtr{}},
		{name: "embedded pointer without inline", doc: &embeddedPtr{Credentials: &Credentials{Login: "l"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			matchesDriver(t, tc.doc)
		})
	}
}

func TestInsertInlineNestedMapValues(t *testing.T) {
	t.Parallel()

	v := &struct {
		Inner struct {
			X int            `bson:"x"`
			M map[string]any `bson:",inline"`
		} `bson:",inline"`
	}{}
	v.Inner.X = 1
	v.Inner.M = map[string]any{"k": "v"}
	matchesDriver(t, v)
}

func TestUpdateInlineOwnFieldPolicyShadows(t *testing.T) {
	t.Parallel()

	type stripped struct {
		Name  string     `bson:"name" behavior:"immutable"`
		Inner inlineMeta `bson:",inline"`
	}
	got := update(t, &stripped{Name: "own", Inner: inlineMeta{Name: "inner", Version: 1}},
		[]behavior.Option{behavior.WithKinds(behavior.DefaultUpdateKinds...)})
	require.Equal(t, bson.M{"$set": bson.M{"version": 1}}, got,
		"a stripped own field keeps shadowing the inline one, which never reaches $set")

	type omitted struct {
		Name  string     `bson:"name,omitonupdate"`
		Inner inlineMeta `bson:",inline"`
	}
	got = update(t, &omitted{Name: "own", Inner: inlineMeta{Name: "inner", Version: 1}}, nil)
	require.Equal(t, bson.M{"$set": bson.M{"version": 1, "owner": ""}}, got)
}

func TestUpdateInlineIDNeverUnset(t *testing.T) {
	t.Parallel()

	type ident struct {
		ID *string `bson:"_id"`
	}
	type doc struct {
		Name string `bson:"name"`
		ID   ident  `bson:",inline"`
	}
	got := update(t, &doc{Name: "n"}, nil)
	require.Equal(t, bson.M{"$set": bson.M{"name": "n"}}, got, "_id is immutable: never in $set or $unset")
}

// Outer embeds Credentials without inline, so the codec stores it as a
// "credentials" subdocument of whatever inlines Outer.
type Outer struct {
	Credentials
	Label string `bson:"label"`
}

func TestEmbeddedUnderInlineAncestor(t *testing.T) {
	t.Parallel()

	type doc struct {
		Outer `bson:",inline"`
		Name  string `bson:"name"`
	}
	matchesDriver(t, &doc{Outer: Outer{Credentials: Credentials{Secret: "s", Login: "l"}, Label: "x"}, Name: "n"})

	got := fieldPaths(t, doc{}, behavior.DefaultResponseKinds)
	require.Equal(t, mongo.FieldPaths{
		Selectable: []string{"credentials.login", "label", "name"},
		Denied:     []string{"credentials.secret"},
	}, got)
}

func TestNilEmbeddedPointerPaths(t *testing.T) {
	t.Parallel()

	type doc struct {
		*Credentials
		Name string `bson:"name"`
	}
	got := fieldPaths(t, doc{}, behavior.DefaultResponseKinds)
	require.Equal(t, mongo.FieldPaths{
		Selectable: []string{"credentials.login", "name"},
		Denied:     []string{"credentials.secret"},
	}, got, "schema-walk still exposes the fields behind a nil embedded pointer")
	require.Equal(t, bson.M{"credentials.secret": 0}, projection(t, doc{}, behavior.DefaultResponseKinds))
}

type guarded struct {
	Owner   string `bson:"owner" behavior:"immutable"`
	Version int    `bson:"version"`
	Note    string `bson:"note,omitonupdate"`
}

func TestUpdateStructValueHonorsStrip(t *testing.T) {
	t.Parallel()

	type doc struct {
		Name    string  `bson:"name"`
		Guarded guarded `bson:"guarded"`
	}
	got := update(t, &doc{Name: "n", Guarded: guarded{Owner: "o", Version: 2, Note: "x"}},
		[]behavior.Option{behavior.WithKinds(behavior.DefaultUpdateKinds...)})
	require.Equal(t, bson.M{"$set": bson.M{"name": "n", "guarded": bson.M{"version": 2}}}, got,
		"a struct value is folded: its immutable and omitonupdate fields stay out of $set")
}

func TestUnexportedEmbeddedTypeSkipped(t *testing.T) {
	t.Parallel()

	type doc struct {
		guarded
		Name string `bson:"name"`
	}
	v := &doc{guarded: guarded{Owner: "o", Version: 2}, Name: "n"}
	matchesDriver(t, v)
	got := update(t, v, nil)
	require.Equal(t, bson.M{"$set": bson.M{"name": "n"}}, got,
		"the codec skips an embedded unexported type, though the engine promotes its exported fields")
}

func TestUpdateEmbeddedSubdocumentHonorsStrip(t *testing.T) {
	t.Parallel()

	type Guarded struct {
		Owner   string `bson:"owner" behavior:"immutable"`
		Version int    `bson:"version"`
	}
	type doc struct {
		Guarded
		Name string `bson:"name"`
	}
	got := update(t, &doc{Guarded: Guarded{Owner: "o", Version: 2}, Name: "n"},
		[]behavior.Option{behavior.WithKinds(behavior.DefaultUpdateKinds...)})
	require.Equal(t, bson.M{"$set": bson.M{"name": "n", "guarded": bson.M{"version": 2}}}, got)
}

// Node embeds itself; promoting its fields would never terminate.
type Node struct {
	*Node `bson:",inline"`
	Name  string `bson:"name"`
}

func TestSelfEmbeddingIsBounded(t *testing.T) {
	t.Parallel()

	eng := behavior.New[bson.M](mongo.NewInsertTranslator())
	_, err := eng.Translate(t.Context(), &Node{Name: "n"})
	require.Error(t, err, "a type inlined into itself is rejected rather than recursing forever")

	type plain struct {
		*plain
		Name string
	}
	_, err = behavior.Resolve(plain{Name: "n"}, behavior.WithSchemaWalk())
	require.NoError(t, err, "the engine walks a self-embedding type under its depth bound")
}

type sized struct {
	N  int64  `bson:"n,minsize"`
	U  uint32 `bson:"u,minsize"`
	B  int64  `bson:"b,minsize"`
	E  []int  `bson:"e"`
	Z  inner0 `bson:"z,omitempty"`
	At int64  `bson:"at"`
}

type inner0 struct {
	X int `bson:"x"`
}

func TestInsertStructValueMatchesDriver(t *testing.T) {
	t.Parallel()

	type doc struct {
		S  sized  `bson:"s"`
		N  int64  `bson:"n,minsize"`
		Ok uint64 `bson:"ok,minsize"`
	}
	matchesDriver(t, &doc{S: sized{N: 1, U: 2, B: 1 << 40, E: []int{}}, N: 7, Ok: 1 << 40})
}

func TestUpdateMinSize(t *testing.T) {
	t.Parallel()

	type doc struct {
		N   int64  `bson:"n,minsize"`
		Big int64  `bson:"big,minsize"`
		U   uint   `bson:"u,minsize"`
		P   int64  `bson:"p"`
		S   sized  `bson:"s"`
		Q   uint64 `bson:"q,minsize"`
	}
	got := update(t, &doc{N: 1, Big: 1 << 40, U: 3, P: 4, S: sized{N: 5}, Q: 1 << 40}, nil)
	set, ok := got["$set"].(bson.M)
	require.True(t, ok)
	require.Equal(t, int32(1), set["n"])
	require.Equal(t, int64(1<<40), set["big"])
	require.Equal(t, int32(3), set["u"])
	require.Equal(t, int64(4), set["p"])
	require.Equal(t, uint64(1<<40), set["q"])
	require.Equal(t, int32(5), set["s"].(bson.M)["n"], "minsize applies inside folded struct values")
}

func TestNilInlineAncestorPaths(t *testing.T) {
	t.Parallel()

	type doc struct {
		*Outer `bson:",inline"`
		Name   string `bson:"name"`
	}
	got := fieldPaths(t, doc{}, behavior.DefaultResponseKinds)
	require.Equal(t, mongo.FieldPaths{
		Selectable: []string{"credentials.login", "label", "name"},
		Denied:     []string{"credentials.secret"},
	}, got, "schema-walk exposes the paths behind a nil inline ancestor")

	matchesDriver(t, &doc{Name: "n"})
}

func TestInsertMinSizeMatchesDriver(t *testing.T) {
	t.Parallel()

	type plain struct {
		N int64  `bson:"n"`
		U uint32 `bson:"u"`
	}
	type doc struct {
		P   *int64           `bson:"p,minsize"`
		L   []int64          `bson:"l,minsize"`
		LL  [][]uint64       `bson:"ll,minsize"`
		M   map[string]int64 `bson:"m,minsize"`
		A   []any            `bson:"a,minsize"`
		S   *plain           `bson:"s,minsize"`
		SS  []plain          `bson:"ss,minsize"`
		Big []int64          `bson:"big,minsize"`
		Off []int64          `bson:"off"`
	}
	matchesDriver(t, &doc{
		P:   new(int64(7)),
		L:   []int64{1, 2},
		LL:  [][]uint64{{3}},
		M:   map[string]int64{"k": 4},
		A:   []any{int64(5), "x"},
		S:   &plain{N: 6, U: 7},
		SS:  []plain{{N: 8}},
		Big: []int64{1 << 40},
		Off: []int64{9},
	})
}

func TestUpdateRejectsDottedInlineKey(t *testing.T) {
	t.Parallel()

	type doc struct {
		Guarded guarded        `bson:"guarded,omitonupdate"`
		Extra   map[string]any `bson:",inline"`
	}
	v := &doc{Extra: map[string]any{"guarded.owner": "changed"}}

	eng := behavior.New[bson.M](mongo.NewUpdateTranslator())
	_, err := eng.Translate(t.Context(), v)
	require.Error(t, err, "a dotted key would become a path into a skipped field")

	matchesDriver(t, v)
}

// code is an integer with a pointer-receiver BSON marshaler that writes a
// string.
type code int64

func (c *code) MarshalBSONValue() (byte, []byte, error) {
	typ, data, err := bson.MarshalValue(fmt.Sprintf("C%d", int64(*c)))
	return byte(typ), data, err
}

func TestInsertMinSizeCustomMarshaler(t *testing.T) {
	t.Parallel()

	type doc struct {
		C code  `bson:"c,minsize"`
		N int64 `bson:"n,minsize"`
	}
	matchesDriver(t, &doc{C: 7, N: 8})
}

func TestInsertUnderStrippedInlineAncestor(t *testing.T) {
	t.Parallel()

	type child struct {
		X int `bson:"x"`
	}
	type meta struct {
		Kids  []child          `bson:"kids"`
		ByKey map[string]child `bson:"by_key"`
	}
	type doc struct {
		Meta meta   `bson:",inline" behavior:"input_only"`
		Name string `bson:"name"`
	}
	v := &doc{Meta: meta{Kids: []child{{X: 1}}, ByKey: map[string]child{"a": {X: 2}}}, Name: "n"}

	// Insert is unfiltered even when the engine is given strip kinds: the
	// stripped inline struct's fields are written as the codec writes them.
	eng := behavior.New[bson.M](mongo.NewInsertTranslator(), behavior.WithKinds(behavior.InputOnly))
	got, err := eng.Translate(t.Context(), v)
	require.NoError(t, err)
	want, err := bson.Marshal(v)
	require.NoError(t, err)
	raw, err := bson.Marshal(got)
	require.NoError(t, err)
	require.Equal(t, decodeM(t, want), decodeM(t, raw))
}

// zeroable reports itself empty through bson.Zeroer.
type zeroable struct {
	V int `bson:"v"`
}

func (z zeroable) IsZero() bool { return z.V < 0 }

// tag has a pointer-receiver BSON marshaler.
type tag struct {
	Name string
}

func (tg *tag) MarshalBSONValue() (byte, []byte, error) {
	typ, data, err := bson.MarshalValue("tag:" + tg.Name)
	return byte(typ), data, err
}

func TestInsertOmitEmptyMatchesDriver(t *testing.T) {
	t.Parallel()

	type plain struct {
		X int `bson:"x"`
	}
	type doc struct {
		S   plain          `bson:"s,omitempty"`
		Z   zeroable       `bson:"z,omitempty"`
		Neg zeroable       `bson:"neg,omitempty"`
		I   any            `bson:"i,omitempty"`
		E   []int          `bson:"e,omitempty"`
		M   map[string]int `bson:"m,omitempty"`
		Str string         `bson:"str,omitempty"`
		P   *plain         `bson:"p,omitempty"`
		Pz  *plain         `bson:"pz,omitempty"`
		Arr [0]int         `bson:"arr,omitempty"`
		N   []int          `bson:"n"`
		Em  map[string]int `bson:"em"`
	}
	matchesDriver(t, &doc{Neg: zeroable{V: -1}, Pz: &plain{}, Em: map[string]int{}})
}

func TestInsertPointerReceiverMarshaler(t *testing.T) {
	t.Parallel()

	type doc struct {
		T   tag            `bson:"t"`
		L   []tag          `bson:"l"`
		M   map[string]tag `bson:"m"`
		Ptr *tag           `bson:"ptr"`
	}
	matchesDriver(t, &doc{
		T:   tag{Name: "a"},
		L:   []tag{{Name: "b"}},
		M:   map[string]tag{"k": {Name: "c"}},
		Ptr: &tag{Name: "d"},
	})
}

// setMatchesDriver asserts that each listed $set value of an update encodes as
// the codec encodes the same field of doc.
func setMatchesDriver(t *testing.T, doc any, keys ...string) {
	t.Helper()

	got := update(t, doc, nil)
	set, ok := got["$set"].(bson.M)
	require.True(t, ok)
	raw, err := bson.Marshal(doc)
	require.NoError(t, err)
	want := decodeM(t, raw)
	for _, k := range keys {
		enc, err := bson.Marshal(bson.M{k: set[k]})
		require.NoError(t, err, k)
		require.Equal(t, want[k], decodeM(t, enc)[k], k)
	}
}

// badCode's pointer-receiver marshaler always fails.
type badCode int

func (*badCode) MarshalBSONValue() (byte, []byte, error) {
	return 0, nil, fmt.Errorf("bad code")
}

// codes is a slice type with its own marshaler.
type codes []int

func (c codes) MarshalBSONValue() (byte, []byte, error) {
	typ, data, err := bson.MarshalValue(fmt.Sprint(len(c)))
	return byte(typ), data, err
}

func TestUpdateLeavesMatchDriver(t *testing.T) {
	t.Parallel()

	type withCode struct {
		C code `bson:"c"`
	}
	type doc struct {
		S  withCode             `bson:"s"`
		M  map[string]withCode  `bson:"m"`
		MP map[string]*withCode `bson:"mp"`
		L  []withCode           `bson:"l"`
		A  [1]code              `bson:"a"`
		D  bson.D               `bson:"d"`
		CS codes                `bson:"cs"`
	}
	setMatchesDriver(t, &doc{
		S:  withCode{C: 7},
		M:  map[string]withCode{"k": {C: 8}},
		MP: map[string]*withCode{"k": {C: 9}},
		L:  []withCode{{C: 10}},
		A:  [1]code{11},
		D:  bson.D{{Key: "x", Value: int32(1)}},
		CS: codes{1, 2},
	}, "s", "m", "mp", "l", "a", "d", "cs")
}

func TestUpdateMarshalerErrorPropagates(t *testing.T) {
	t.Parallel()

	type doc struct {
		B badCode `bson:"b"`
	}
	_, driverErr := bson.Marshal(&doc{B: 1})
	require.Error(t, driverErr)

	eng := behavior.New[bson.M](mongo.NewUpdateTranslator())
	_, err := eng.Translate(t.Context(), &doc{B: 1})
	require.Error(t, err, "a failing marshaler is reported, not replaced by the raw integer")
}

// Loop is inlined into itself; the codec would recurse forever on it.
type Loop struct {
	*Loop `bson:",inline"`
	N     int `bson:"n"`
}

// dupKeys has two same-depth fields named "a"; the codec rejects it.
type dupKeys struct {
	A string `bson:"a"`
	B string `bson:"a"`
}

// failing has a value-receiver marshaler that always fails.
type failing struct{}

func (failing) MarshalBSONValue() (byte, []byte, error) { return 0, nil, fmt.Errorf("failing") }

func TestInsertValidationFollowsEncodedValues(t *testing.T) {
	t.Parallel()

	type viaInterface struct {
		V any `bson:"v"`
	}
	eng := behavior.New[bson.M](mongo.NewInsertTranslator())
	_, err := eng.Translate(t.Context(), &viaInterface{V: &Loop{}})
	require.Error(t, err, "a self-inlined type behind an interface is rejected, not handed to the codec")

	type ignored struct {
		Skip dupKeys  `bson:"-"`
		Nil  *dupKeys `bson:"nil"`
		Name string   `bson:"name"`
	}
	matchesDriver(t, &ignored{Name: "n"})
}

func TestUpdateSliceUnderMapValue(t *testing.T) {
	t.Parallel()

	type withCodes struct {
		C []code `bson:"c"`
	}
	type doc struct {
		M map[string]withCodes `bson:"m"`
	}
	setMatchesDriver(t, &doc{M: map[string]withCodes{"k": {C: []code{7}}}}, "m")
}

func TestUpdateValueMarshalerErrorPropagates(t *testing.T) {
	t.Parallel()

	type doc struct {
		F failing  `bson:"f"`
		P *failing `bson:"p"`
	}
	for _, v := range []*doc{{}, {P: &failing{}}} {
		_, driverErr := bson.Marshal(v)
		require.Error(t, driverErr)
		eng := behavior.New[bson.M](mongo.NewUpdateTranslator())
		_, err := eng.Translate(t.Context(), v)
		require.Error(t, err)
	}
}

// SelfMarshal is inlined into itself and has a pointer-only marshaler, which
// the codec uses only for an addressable value.
type SelfMarshal struct {
	*SelfMarshal `bson:",inline"`
	N            int `bson:"n"`
}

func (*SelfMarshal) MarshalBSONValue() (byte, []byte, error) {
	typ, data, err := bson.MarshalValue("self")
	return byte(typ), data, err
}

func TestInsertValidationRespectsMarshalerSelection(t *testing.T) {
	t.Parallel()

	type doc struct {
		V any `bson:"v"`
	}
	eng := behavior.New[bson.M](mongo.NewInsertTranslator())
	_, err := eng.Translate(t.Context(), &doc{V: SelfMarshal{}})
	require.Error(t, err, "a non-addressable value skips the pointer marshaler, so its layout is validated")

	type omitted struct {
		Z    zeroDup `bson:"z,omitempty"`
		Name string  `bson:"name"`
	}
	matchesDriver(t, &omitted{Name: "n"})
}

// zeroDup has duplicate keys but reports itself empty, so omitempty drops it.
type zeroDup struct {
	A string `bson:"a"`
	B string `bson:"a"`
}

func (zeroDup) IsZero() bool { return true }

func TestUpdatePointerUnderMapValue(t *testing.T) {
	t.Parallel()

	type withCode struct {
		C code `bson:"c"`
	}
	type inner struct {
		C code `bson:"ic"`
	}
	type outer struct {
		P      *withCode `bson:"p"`
		*inner `bson:",inline"`
	}
	type doc struct {
		M map[string]outer `bson:"m"`
	}
	setMatchesDriver(t, &doc{M: map[string]outer{"k": {P: &withCode{C: 7}, inner: &inner{C: 8}}}}, "m")
}

func TestUpdateStripsInsideCollectionShapes(t *testing.T) {
	t.Parallel()

	type item struct {
		Owner string `bson:"owner" behavior:"immutable"`
		Note  string `bson:"note,omitonupdate"`
		V     int    `bson:"v"`
	}
	type doc struct {
		Arr    [1]item           `bson:"arr"`
		PtrS   *[]item           `bson:"ptrs"`
		Nested [][]item          `bson:"nested"`
		ByKey  map[string][]item `bson:"by_key"`
		PtrEl  []*item           `bson:"ptrel"`
		Bytes  [2]byte           `bson:"bytes"`
	}
	it := item{Owner: "o", Note: "n", V: 1}
	got := update(t, &doc{
		Arr:    [1]item{it},
		PtrS:   &[]item{it},
		Nested: [][]item{{it}},
		ByKey:  map[string][]item{"k": {it}},
		PtrEl:  []*item{&it},
		Bytes:  [2]byte{1, 2},
	}, []behavior.Option{behavior.WithKinds(behavior.DefaultUpdateKinds...)})

	clean := bson.M{"v": 1}
	set := got["$set"].(bson.M)
	require.Equal(t, bson.A{clean}, set["arr"])
	require.Equal(t, bson.A{clean}, set["ptrs"])
	require.Equal(t, bson.A{bson.A{clean}}, set["nested"])
	require.Equal(t, bson.M{"k": bson.A{clean}}, set["by_key"])
	require.Equal(t, bson.A{clean}, set["ptrel"])
	require.Equal(t, [2]byte{1, 2}, set["bytes"])
}

func TestUpdateCodecTypesStayLeaves(t *testing.T) {
	t.Parallel()

	type doc struct {
		TS  bson.Timestamp   `bson:"ts"`
		Bin bson.Binary      `bson:"bin"`
		U   url.URL          `bson:"u"`
		E   [1]bson.E        `bson:"e"`
		LM  []map[string]int `bson:"lm"`
	}
	setMatchesDriver(t, &doc{
		TS:  bson.Timestamp{T: 1, I: 2},
		Bin: bson.Binary{Subtype: 0, Data: []byte{1}},
		U:   url.URL{Scheme: "https", Host: "x"},
		E:   [1]bson.E{{Key: "x", Value: int32(1)}},
		LM:  []map[string]int{{"a": 1}},
	}, "ts", "bin", "u", "e", "lm")

	type bad struct {
		LM []map[string]int `bson:"lm"`
	}
	eng := behavior.New[bson.M](mongo.NewUpdateTranslator())
	_, err := eng.Translate(t.Context(), &bad{LM: []map[string]int{{"$bad": 1}}})
	require.Error(t, err, "maps nested in unresolved elements still reject operator keys")

	got := fieldPaths(t, doc{}, behavior.DefaultResponseKinds)
	require.Equal(t, []string{"bin", "e", "lm", "ts", "u"}, got.Selectable, "no paths inside codec value types")

	type layered struct {
		P  *bson.Timestamp         `bson:"p"`
		L  []bson.Timestamp        `bson:"l"`
		M  map[string]*bson.Binary `bson:"m"`
		LL [][]bson.Timestamp      `bson:"ll"`
	}
	got = fieldPaths(t, layered{}, behavior.DefaultResponseKinds)
	require.Equal(t, []string{"l", "ll", "m", "p"}, got.Selectable, "nor through pointer and collection layers")
}

func TestCodecTagAndTypeEdges(t *testing.T) {
	t.Parallel()

	type doc struct {
		E    bson.E `bson:"e"`
		Dash string `bson:"-,omitempty"`
		Skip string `bson:"-"`
	}
	v := &doc{E: bson.E{Key: "k", Value: "v"}, Dash: "d", Skip: "s"}
	matchesDriver(t, v)
	setMatchesDriver(t, v, "e", "-")

	got := fieldPaths(t, doc{}, behavior.DefaultResponseKinds)
	require.Equal(t, []string{"-", "e", "e.key", "e.value"}, got.Selectable,
		"bson.E is an ordinary struct; \"-,omitempty\" names the field \"-\"")
}

func TestCodecTagForms(t *testing.T) {
	t.Parallel()

	type meta struct {
		Secret string `bson:"secret" behavior:"input_only"`
		V      int    `bson:"v"`
	}
	// The legacy colon-free tag cannot be written as a literal without
	// tripping go vet's structtag check, so the type is built by reflection.
	docType := reflect.StructOf([]reflect.StructField{
		{Name: "Meta", Type: reflect.TypeFor[meta](), Tag: `bson:"inline"`},
		{Name: "Bare", Type: reflect.TypeFor[string](), Tag: "private"},
		{Name: "Option", Type: reflect.TypeFor[string](), Tag: `bson:"omitempty"`},
	})
	v := reflect.New(docType)
	v.Elem().Field(0).Set(reflect.ValueOf(meta{Secret: "s", V: 1}))
	v.Elem().Field(1).SetString("b")
	matchesDriver(t, v.Interface())
	setMatchesDriver(t, v.Interface(), "v", "private")

	got := fieldPaths(t, reflect.Zero(docType).Interface(), behavior.DefaultResponseKinds)
	require.Equal(t, mongo.FieldPaths{Selectable: []string{"omitempty", "private", "v"}, Denied: []string{"secret"}}, got,
		"a first-token option applies; a bare tag names the field")
}

func TestInsertKeepsNestedOrder(t *testing.T) {
	t.Parallel()

	type doc struct {
		D bson.D `bson:"d"`
	}
	got := insert(t, &doc{D: bson.D{{Key: "b", Value: int32(1)}, {Key: "a", Value: int32(2)}}})
	require.Equal(t, bson.D{{Key: "b", Value: int32(1)}, {Key: "a", Value: int32(2)}}, got["d"])
}

func TestDottedNamesNotAddressable(t *testing.T) {
	t.Parallel()

	type meta struct {
		Dotted string `bson:"x.y"`
	}
	type doc struct {
		Name   string `bson:"name"`
		Dotted string `bson:"a.b" behavior:"input_only"`
	}
	type inlined struct {
		Meta meta `bson:",inline"`
	}

	matchesDriver(t, &doc{Name: "n", Dotted: "d"})

	upd := behavior.New[bson.M](mongo.NewUpdateTranslator())
	_, err := upd.Translate(t.Context(), &doc{Name: "n", Dotted: "d"})
	require.ErrorIs(t, err, mongo.ErrUnaddressableName)
	_, err = upd.Translate(t.Context(), &inlined{Meta: meta{Dotted: "d"}})
	require.ErrorIs(t, err, mongo.ErrUnaddressableName)

	paths := behavior.New[mongo.FieldPaths](mongo.NewFieldPathsTranslator(),
		behavior.WithKinds(behavior.DefaultResponseKinds...), behavior.WithSchemaWalk())
	_, err = paths.Translate(t.Context(), doc{})
	require.ErrorIs(t, err, mongo.ErrUnaddressableName)
}

// ptrMarshaled has a pointer-only marshaler and a field update must strip
// when the codec falls back to its ordinary encoding.
type ptrMarshaled struct {
	Owner string `bson:"owner" behavior:"immutable"`
	V     int    `bson:"v"`
}

func (*ptrMarshaled) MarshalBSONValue() (byte, []byte, error) {
	typ, data, err := bson.MarshalValue("marshaled")
	return byte(typ), data, err
}

func TestPointerOnlyMarshalerFallback(t *testing.T) {
	t.Parallel()

	type doc struct {
		Direct ptrMarshaled            `bson:"direct"`
		ByKey  map[string]ptrMarshaled `bson:"by_key"`
	}
	v := &doc{Direct: ptrMarshaled{Owner: "o", V: 1}, ByKey: map[string]ptrMarshaled{"k": {Owner: "o", V: 2}}}

	got := update(t, v, []behavior.Option{behavior.WithKinds(behavior.DefaultUpdateKinds...)})
	set := got["$set"].(bson.M)
	direct := set["direct"].(bson.RawValue)
	require.Equal(t, "marshaled", direct.StringValue(), "addressable: the codec uses the marshaler")
	require.Equal(t, bson.M{"k": bson.M{"v": 2}}, set["by_key"],
		"a map value is not addressable: the codec encodes its fields, so the immutable owner is stripped")

	paths := fieldPaths(t, doc{}, []behavior.Kind{behavior.Immutable})
	require.Contains(t, paths.Denied, "by_key", "either encoding may store the stripped field: deny the whole field")
}

// Blob is a byte slice with a pointer-only marshaler.
type Blob []byte

func (*Blob) MarshalBSONValue() (byte, []byte, error) {
	typ, data, err := bson.MarshalValue("blob")
	return byte(typ), data, err
}

// Doc is a bson.D with a pointer-only marshaler.
type Doc bson.D

func (*Doc) MarshalBSONValue() (byte, []byte, error) {
	typ, data, err := bson.MarshalValue("doc")
	return byte(typ), data, err
}

func TestPointerOnlyMarshalerFallbackKeepsBuiltinRules(t *testing.T) {
	t.Parallel()

	type doc struct {
		B map[string]Blob `bson:"b"`
		D map[string]Doc  `bson:"d"`
	}
	setMatchesDriver(t, &doc{
		B: map[string]Blob{"k": {1, 2}},
		D: map[string]Doc{"k": {{Key: "x", Value: int32(1)}}},
	}, "b", "d")
}

// Chain is a recursive type; a cyclic value must fail, not crash the codec.
type Chain struct {
	Next *Chain `bson:"next"`
	N    int    `bson:"n"`
}

func TestInsertRejectsCyclicValue(t *testing.T) {
	t.Parallel()

	c := &Chain{N: 1}
	c.Next = c
	eng := behavior.New[bson.M](mongo.NewInsertTranslator())
	_, err := eng.Translate(t.Context(), c)
	require.Error(t, err, "a cyclic value is rejected by the depth bound")

	matchesDriver(t, &Chain{N: 1, Next: &Chain{N: 2}})
}

func TestInsertRejectsOperatorKeyInCodeScope(t *testing.T) {
	t.Parallel()

	type doc struct {
		C bson.CodeWithScope `bson:"c"`
	}
	eng := behavior.New[bson.M](mongo.NewInsertTranslator())
	_, err := eng.Translate(t.Context(), &doc{C: bson.CodeWithScope{Code: "x", Scope: bson.M{"$bad": 1}}})
	require.Error(t, err)

	matchesDriver(t, &doc{C: bson.CodeWithScope{Code: "x", Scope: bson.M{"ok": int32(1)}}})
}

func TestRawValueIsLeaf(t *testing.T) {
	t.Parallel()

	rv := bson.RawValue{Type: bson.TypeString, Value: bsoncore.AppendString(nil, "x")}
	type doc struct {
		R bson.RawValue `bson:"r"`
	}
	setMatchesDriver(t, &doc{R: rv}, "r")
	require.Equal(t, []string{"r"}, fieldPaths(t, doc{}, behavior.DefaultResponseKinds).Selectable)
}
