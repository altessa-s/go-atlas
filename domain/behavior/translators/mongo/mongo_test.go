// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/domain/behavior"

	mongo "github.com/altessa-s/go-atlas/domain/behavior/translators/mongo"
)

type address struct {
	City   string `bson:"city"`
	Secret string `bson:"secret" behavior:"input_only"`
}

type user struct {
	ID         string            `bson:"_id" behavior:"identifier"`
	Name       string            `bson:"name"`
	Nick       *string           `bson:"nick,omitempty"`
	Active     bool              `bson:"active"`
	Tags       []string          `bson:"tags"`
	Blob       []byte            `bson:"blob"`
	CreateTime time.Time         `bson:"create_time" behavior:"output_only"`
	TenantID   string            `bson:"tenant_id" behavior:"immutable"`
	Password   string            `bson:"password" behavior:"input_only"`
	Home       *address          `bson:"home"`
	Aliases    []address         `bson:"aliases"`
	Labels     map[string]string `bson:"labels"`
}

// insert builds an insert document through the behavior engine (no WithKinds, so
// nothing is stripped).
func insert(t *testing.T, v any, opts ...mongo.Option) bson.M {
	t.Helper()
	eng := behavior.New[bson.M](mongo.NewInsertTranslator(opts...))
	got, err := eng.Translate(t.Context(), v)
	require.NoError(t, err)
	return got
}

// update builds an update document through the behavior engine with the given
// behavior options (kinds, tag name, …).
func update(t *testing.T, v any, behOpts []behavior.Option, opts ...mongo.Option) bson.M {
	t.Helper()
	eng := behavior.New[bson.M](mongo.NewUpdateTranslator(opts...), behOpts...)
	got, err := eng.Translate(t.Context(), v)
	require.NoError(t, err)
	return got
}

func TestInsertTranslator(t *testing.T) {
	t.Parallel()

	ct := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	u := user{
		ID:         "u1",
		Name:       "Ann",
		Active:     true,
		Tags:       []string{"a", "b"},
		Blob:       []byte{0x01, 0x02},
		CreateTime: ct,
		TenantID:   "t1",
		Password:   "secret",
		Home:       &address{City: "NYC", Secret: "s"},
		Aliases:    []address{{City: "LA", Secret: "x"}},
		Labels:     map[string]string{"k": "v"},
	}

	got := insert(t, &u)

	// The insert document is the codec's encoding of u, decoded: values carry
	// their BSON Go types (Binary, DateTime) and every field is present.
	want := bson.M{
		"_id":         "u1",
		"name":        "Ann",
		"active":      true,
		"tags":        bson.A{"a", "b"},
		"blob":        bson.Binary{Data: []byte{0x01, 0x02}},
		"create_time": bson.NewDateTimeFromTime(ct),
		"tenant_id":   "t1",
		"password":    "secret",
		"home":        bson.D{{Key: "city", Value: "NYC"}, {Key: "secret", Value: "s"}},
		"aliases":     bson.A{bson.D{{Key: "city", Value: "LA"}, {Key: "secret", Value: "x"}}},
		"labels":      bson.D{{Key: "k", Value: "v"}},
	}
	require.Equal(t, want, got)
	matchesDriver(t, &u)
}

func TestInsertTranslatorOmitemptyAndEmptyCollections(t *testing.T) {
	t.Parallel()

	// Nick is a nil pointer with omitempty -> absent. Nil collections without
	// omitempty are written as null, as the codec writes them; empty non-nil
	// ones as [] / {}.
	got := insert(t, &user{Name: "Bob"})

	require.NotContains(t, got, "nick")
	for _, k := range []string{"tags", "blob", "aliases", "labels"} {
		require.Contains(t, got, k)
	}
	matchesDriver(t, &user{Name: "Bob"})
	matchesDriver(t, &user{Name: "Bob", Tags: []string{}, Aliases: []address{}, Labels: map[string]string{}})
	// A nil pointer-to-struct without omitempty is written as null.
	require.Contains(t, got, "home")
	require.Nil(t, got["home"])
	require.Equal(t, "Bob", got["name"])
	require.Equal(t, false, got["active"])

	// time.Time is written as a BSON datetime, not recursed.
	require.IsType(t, bson.DateTime(0), got["create_time"])
}

func TestInsertTranslatorOmitemptyPresentPointer(t *testing.T) {
	t.Parallel()

	got := insert(t, &user{Name: "Cy", Nick: new("cy")})
	require.Equal(t, "cy", got["nick"], "the codec writes the pointer's target")
}

func TestInsertTranslatorOmitemptyZeroValues(t *testing.T) {
	t.Parallel()

	type doc struct {
		Count int       `bson:"count,omitempty"`
		Name  string    `bson:"name,omitempty"`
		On    bool      `bson:"on,omitempty"`
		At    time.Time `bson:"at,omitempty"`
		Nick  *string   `bson:"nick,omitempty"`
	}

	// omitempty matches the BSON driver: every zero value is omitted on insert.
	got := insert(t, &doc{})
	require.Empty(t, got)

	// A non-nil pointer is not zero, even when it points at a zero value.
	got = insert(t, &doc{Count: 1, Nick: new("")})
	require.Equal(t, bson.M{"count": int32(1), "nick": ""}, got)
}

func TestInsertTranslatorByteSliceIsOpaque(t *testing.T) {
	t.Parallel()

	got := insert(t, &user{Name: "Di", Blob: []byte{0xDE, 0xAD}})
	require.Equal(t, bson.Binary{Data: []byte{0xDE, 0xAD}}, got["blob"])
}

func TestUpdateTranslator(t *testing.T) {
	t.Parallel()

	ct := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	u := user{
		ID:         "u1", // removed from $set
		Name:       "Ann",
		Active:     true,
		Tags:       []string{"a"},
		CreateTime: ct,   // output_only -> excluded
		TenantID:   "t1", // immutable -> excluded
		Password:   "p",  // input_only -> kept (not in update strip set)
		Home:       &address{City: "NYC"},
	}

	got := update(t, &u, []behavior.Option{behavior.WithKinds(behavior.DefaultUpdateKinds...)})

	set, ok := got["$set"].(bson.M)
	require.True(t, ok)
	require.NotContains(t, set, "_id")
	require.NotContains(t, set, "create_time")
	require.NotContains(t, set, "tenant_id")
	require.Equal(t, "Ann", set["name"])
	require.Equal(t, true, set["active"])
	require.Equal(t, bson.A{"a"}, set["tags"])
	require.Equal(t, "p", set["password"])
	// Nested full-replace: whole subdocument in $set, no nested $unset.
	require.Equal(t, bson.M{"city": "NYC", "secret": ""}, set["home"])

	// Empty collections and nil pointers go to $unset.
	unset, ok := got["$unset"].(bson.M)
	require.True(t, ok)
	require.Contains(t, unset, "nick")    // nil *string
	require.Contains(t, unset, "blob")    // empty []byte
	require.Contains(t, unset, "aliases") // empty slice
	require.Contains(t, unset, "labels")  // empty map
	require.Nil(t, unset["nick"])
}

func TestUpdateTranslatorExcludesIdentifier(t *testing.T) {
	t.Parallel()

	// An identifier stored under a name other than "_id" stays out of the update
	// document whatever kinds the engine strips: it selects the record, it is
	// never written. DefaultUpdateKinds (the documented invocation) keeps the
	// identifier in the payload, so the translator has to exclude it itself.
	type doc struct {
		UID      string  `bson:"uid" behavior:"identifier"`
		Ref      *string `bson:"ref" behavior:"identifier"`
		TenantID string  `bson:"tenant_id" behavior:"immutable"`
		Name     string  `bson:"name"`
	}
	tests := []struct {
		name  string
		kinds []behavior.Kind
	}{
		{name: "default update kinds", kinds: behavior.DefaultUpdateKinds},
		{name: "no kinds", kinds: nil},
		{name: "identifier stripped by the engine", kinds: []behavior.Kind{behavior.Identifier, behavior.Immutable}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := []behavior.Option{behavior.WithKinds(tt.kinds...)}
			// A set identifier never enters $set, a nil identifier pointer
			// never enters $unset.
			got := update(t, &doc{UID: "u1", Name: "n"}, opts)
			set := got["$set"].(bson.M)
			unset, _ := got["$unset"].(bson.M)
			require.NotContains(t, set, "uid")
			require.NotContains(t, unset, "ref")
			require.Equal(t, "n", set["name"])

			got = update(t, &doc{UID: "u1", Ref: new("r1"), Name: "n"}, opts)
			set = got["$set"].(bson.M)
			require.NotContains(t, set, "uid")
			require.NotContains(t, set, "ref")
		})
	}

	got := update(t, &doc{UID: "u1", TenantID: "t1", Name: "n"},
		[]behavior.Option{behavior.WithKinds(behavior.DefaultUpdateKinds...)})
	require.Equal(t, bson.M{"$set": bson.M{"name": "n"}}, got)

	got = update(t, &user{ID: "u1", Name: "Ann"},
		[]behavior.Option{behavior.WithKinds(behavior.DefaultUpdateKinds...)})
	set := got["$set"].(bson.M)
	require.NotContains(t, set, "_id")
	if unset, ok := got["$unset"].(bson.M); ok {
		require.NotContains(t, unset, "_id")
	}

	// An identifier inside a collection element is the element's own: the
	// replaced collection keeps it.
	type item struct {
		ID   string `bson:"id" behavior:"identifier"`
		Name string `bson:"name"`
	}
	type list struct {
		Items  []item          `bson:"items"`
		ByCode map[string]item `bson:"by_code"`
	}
	got = update(t, &list{Items: []item{{ID: "i1", Name: "a"}}, ByCode: map[string]item{"c": {ID: "i2", Name: "b"}}},
		[]behavior.Option{behavior.WithKinds(behavior.DefaultUpdateKinds...)})
	require.Equal(t, bson.M{"$set": bson.M{
		"items":   bson.A{bson.M{"id": "i1", "name": "a"}},
		"by_code": bson.M{"c": bson.M{"id": "i2", "name": "b"}},
	}}, got)
}

func TestUpdateTranslatorNoUnset(t *testing.T) {
	t.Parallel()

	type flat struct {
		Name string `bson:"name"`
	}
	got := update(t, &flat{Name: "x"}, []behavior.Option{behavior.WithKinds(behavior.DefaultUpdateKinds...)})
	require.NotContains(t, got, "$unset")
	require.Equal(t, bson.M{"name": "x"}, got["$set"])
}

func TestUpdateTranslatorKindsOverride(t *testing.T) {
	t.Parallel()

	// Strip only InputOnly, so immutable/output_only are kept; the identifier
	// is excluded by the translator whatever the kinds. Home and Aliases hold
	// an input_only field now stripped: a nil Home would delete its stored
	// value, so it is sent and written by path, and the aliases (a replaced
	// collection carrying it) are left out.
	type account struct {
		ID       string   `bson:"_id" behavior:"identifier"`
		Name     string   `bson:"name"`
		TenantID string   `bson:"tenant_id" behavior:"immutable"`
		Password string   `bson:"password" behavior:"input_only"`
		Home     *address `bson:"home"`
	}
	kinds := []behavior.Option{behavior.WithKinds(behavior.InputOnly)}
	_, err := behavior.New[bson.M](mongo.NewUpdateTranslator(), kinds...).Translate(t.Context(), &account{ID: "u1"})
	require.ErrorIs(t, err, mongo.ErrProtectedUnset, "a nil home would delete the stored secret")

	u := account{ID: "u1", TenantID: "t1", Password: "p", Name: "n", Home: &address{City: "c"}}
	got := update(t, &u, kinds)

	set := got["$set"].(bson.M)
	require.NotContains(t, set, "_id") // _id always removed
	require.Equal(t, "t1", set["tenant_id"])
	require.NotContains(t, set, "password") // input_only now stripped
}

func TestUpdateTranslatorOmitOnUpdate(t *testing.T) {
	t.Parallel()

	type doc struct {
		Name    string `bson:"name"`
		Created string `bson:"created,omitonupdate"`
	}
	got := update(t, &doc{Name: "n", Created: "c"}, []behavior.Option{behavior.WithKinds(behavior.DefaultUpdateKinds...)})
	set := got["$set"].(bson.M)
	require.Contains(t, set, "name")
	require.NotContains(t, set, "created")
}

// projection builds a projection through the behavior engine. Schema-walk is
// mandatory so nested paths appear from the type even for a zero-value carrier.
func projection(t *testing.T, v any, kinds []behavior.Kind, opts ...mongo.Option) bson.M {
	t.Helper()
	eng := behavior.New[bson.M](mongo.NewProjectionTranslator(opts...),
		behavior.WithKinds(kinds...), behavior.WithSchemaWalk())
	got, err := eng.Translate(t.Context(), v)
	require.NoError(t, err)
	return got
}

func TestProjectionTranslator(t *testing.T) {
	t.Parallel()

	// Zero value as type carrier: Home is nil and Aliases empty, yet schema-walk
	// surfaces their nested input_only paths.
	got := projection(t, user{}, behavior.DefaultResponseKinds)

	want := bson.M{
		"password":       0,
		"home.secret":    0,
		"aliases.secret": 0,
	}
	require.Equal(t, want, got)
}

func TestProjectionTranslatorCustomKinds(t *testing.T) {
	t.Parallel()

	got := projection(t, user{}, []behavior.Kind{behavior.Immutable, behavior.Identifier})
	require.Equal(t, bson.M{"_id": 0, "tenant_id": 0}, got)
}

func TestProjectionTranslatorEmpty(t *testing.T) {
	t.Parallel()

	type clean struct {
		Name string `bson:"name"`
	}
	got := projection(t, clean{}, behavior.DefaultResponseKinds)
	require.Equal(t, bson.M{}, got)
}

func TestProjectionTranslatorMapOfStructs(t *testing.T) {
	t.Parallel()

	type doc struct {
		Labels map[string]address `bson:"labels"`
	}
	// Nil map, yet schema-walk resolves the value type. Map values sit under
	// dynamic keys (labels.<key>.secret), which no path can address, so the
	// whole map is excluded rather than leaking the secret.
	got := projection(t, doc{}, behavior.DefaultResponseKinds)
	require.Equal(t, bson.M{"labels": 0}, got)
}

func TestProjectionTranslatorNestedCollections(t *testing.T) {
	t.Parallel()

	type doc struct {
		Grid  [][]address          `bson:"grid"`
		ByKey map[string][]address `bson:"by_key"`
	}

	// Zero value as type carrier: schema-walk unwraps the nested collection
	// layers down to the struct element and emits an unindexed path through
	// arrays; a map layer has dynamic keys, so the map is excluded whole.
	got := projection(t, doc{}, behavior.DefaultResponseKinds)
	want := bson.M{"grid.secret": 0, "by_key": 0}
	require.Equal(t, want, got)

	// Populated values resolve through synthetic wrapper objects and must emit
	// the same paths.
	got = projection(t, doc{
		Grid:  [][]address{{{}}},
		ByKey: map[string][]address{"a": {{}}},
	}, behavior.DefaultResponseKinds)
	require.Equal(t, want, got)
}

func TestInsertTranslatorMapOfStructs(t *testing.T) {
	t.Parallel()

	type doc struct {
		Labels map[string]address `bson:"labels"`
	}
	// Insert is unfiltered: behavior-tagged fields inside map values are written.
	got := insert(t, &doc{Labels: map[string]address{"a": {City: "NYC", Secret: "s"}}})
	require.Equal(t, bson.M{"labels": bson.D{{Key: "a", Value: bson.D{
		{Key: "city", Value: "NYC"}, {Key: "secret", Value: "s"},
	}}}}, got)
}

func TestUpdateTranslatorRejectsStripInsideCollections(t *testing.T) {
	t.Parallel()

	type item struct {
		Name    string `bson:"name"`
		Created string `bson:"created" behavior:"output_only"`
	}
	type doc struct {
		ByKey map[string]item  `bson:"by_key,omitempty"`
		ByPtr map[string]*item `bson:"by_ptr,omitempty"`
		Items []item           `bson:"items,omitempty"`
	}

	// Map values (plain and pointer) and slice elements are replaced with
	// their collection, which cannot keep the stored output_only field: the
	// update is rejected rather than silently dropping it.
	eng := behavior.New[bson.M](mongo.NewUpdateTranslator(), behavior.WithKinds(behavior.DefaultUpdateKinds...))
	for name, d := range map[string]*doc{
		"map value":         {ByKey: map[string]item{"a": {Name: "n", Created: "c"}}},
		"pointer map value": {ByPtr: map[string]*item{"b": {Name: "n", Created: "c"}}},
		"slice element":     {Items: []item{{Name: "n", Created: "c"}}},
	} {
		_, err := eng.Translate(t.Context(), d)
		require.ErrorIs(t, err, mongo.ErrProtectedInCollection, name)
	}
}

func TestMapDollarKeyRejected(t *testing.T) {
	t.Parallel()

	type doc struct {
		Labels map[string]any `bson:"labels"`
	}
	eng := behavior.New[bson.M](mongo.NewInsertTranslator())
	_, err := eng.Translate(t.Context(), &doc{Labels: map[string]any{"$set": 1}})
	require.Error(t, err)
	require.ErrorContains(t, err, "$")
}

func TestMapNonStringKey(t *testing.T) {
	t.Parallel()

	type doc struct {
		M map[int]string `bson:"m"`
	}
	v := &doc{M: map[int]string{1: "a"}}
	matchesDriver(t, v)

	eng := behavior.New[bson.M](mongo.NewUpdateTranslator())
	_, err := eng.Translate(t.Context(), v)
	require.ErrorContains(t, err, "map key must be string", "update folds maps and needs string keys")
}

func TestCustomTagNames(t *testing.T) {
	t.Parallel()

	type doc struct {
		ID   string `db:"_id" beh:"identifier"`
		Name string `db:"name"`
	}
	d := doc{ID: "x", Name: "n"}

	// bson tag name is the translator's concern; behavior tag name belongs to the
	// engine and is set via behavior.WithTagName.
	ins, err := behavior.New[bson.M](mongo.NewInsertTranslator(mongo.WithBsonTagName("db")),
		behavior.WithTagName("beh")).Translate(t.Context(), &d)
	require.NoError(t, err)
	require.Equal(t, bson.M{"_id": "x", "name": "n"}, ins)

	upd := update(t, &d,
		[]behavior.Option{behavior.WithKinds(behavior.DefaultUpdateKinds...), behavior.WithTagName("beh")},
		mongo.WithBsonTagName("db"))
	set := upd["$set"].(bson.M)
	require.NotContains(t, set, "_id")
	require.Equal(t, "n", set["name"])
}

func TestFallbackFieldName(t *testing.T) {
	t.Parallel()

	type doc struct {
		FullName string // no bson tag -> lowercased "fullname"
	}
	got := insert(t, &doc{FullName: "Ann"})
	require.Equal(t, bson.M{"fullname": "Ann"}, got)
}

func TestDashSkipped(t *testing.T) {
	t.Parallel()

	type doc struct {
		Name   string `bson:"name"`
		Hidden string `bson:"-"`
	}
	got := insert(t, &doc{Name: "n", Hidden: "h"})
	require.Equal(t, bson.M{"name": "n"}, got)
}

func TestNilInputRejected(t *testing.T) {
	t.Parallel()

	eng := behavior.New[bson.M](mongo.NewInsertTranslator())
	_, err := eng.Translate(t.Context(), nil)
	require.Error(t, err)
}
