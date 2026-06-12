// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"
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

func ptr[T any](v T) *T { return &v }

// insert builds an insert document through the behavior engine (no WithKinds, so
// nothing is stripped).
func insert(t *testing.T, v any, opts ...mongo.Option) bson.M {
	t.Helper()
	eng := behavior.New[bson.M](mongo.NewInsertTranslator(opts...))
	got, err := eng.Translate(context.Background(), v)
	require.NoError(t, err)
	return got
}

// update builds an update document through the behavior engine with the given
// behavior options (kinds, tag name, …).
func update(t *testing.T, v any, behOpts []behavior.Option, opts ...mongo.Option) bson.M {
	t.Helper()
	eng := behavior.New[bson.M](mongo.NewUpdateTranslator(opts...), behOpts...)
	got, err := eng.Translate(context.Background(), v)
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

	want := bson.M{
		"_id":         "u1",
		"name":        "Ann",
		"active":      true,
		"tags":        bson.A{"a", "b"},
		"blob":        []byte{0x01, 0x02},
		"create_time": ct,
		"tenant_id":   "t1",
		"password":    "secret",
		"home":        bson.M{"city": "NYC", "secret": "s"},
		"aliases":     bson.A{bson.M{"city": "LA", "secret": "x"}},
		"labels":      bson.M{"k": "v"},
	}
	require.Equal(t, want, got)
}

func TestInsertTranslatorOmitemptyAndEmptyCollections(t *testing.T) {
	t.Parallel()

	// Nick is nil pointer with omitempty -> absent. Empty slices/maps absent.
	got := insert(t, &user{Name: "Bob"})

	require.NotContains(t, got, "nick")
	require.NotContains(t, got, "tags")
	require.NotContains(t, got, "blob")
	require.NotContains(t, got, "aliases")
	require.NotContains(t, got, "labels")
	// A nil pointer-to-struct without omitempty is written as a nil value on
	// insert (it falls through to the default case).
	require.Contains(t, got, "home")
	require.Equal(t, (*address)(nil), got["home"])
	require.Equal(t, "Bob", got["name"])
	require.Equal(t, false, got["active"])

	// time.Time is an opaque leaf: written as value, not recursed.
	require.IsType(t, time.Time{}, got["create_time"])
}

func TestInsertTranslatorOmitemptyPresentPointer(t *testing.T) {
	t.Parallel()

	got := insert(t, &user{Name: "Cy", Nick: ptr("cy")})
	require.Equal(t, ptr("cy"), got["nick"])
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
	got = insert(t, &doc{Count: 1, Nick: ptr("")})
	require.Equal(t, bson.M{"count": 1, "nick": ptr("")}, got)
}

func TestInsertTranslatorByteSliceIsOpaque(t *testing.T) {
	t.Parallel()

	got := insert(t, &user{Name: "Di", Blob: []byte{0xDE, 0xAD}})
	require.Equal(t, []byte{0xDE, 0xAD}, got["blob"])
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

	// Strip only InputOnly, so identifier/immutable/output_only are kept.
	u := user{ID: "u1", TenantID: "t1", Password: "p", Name: "n"}
	got := update(t, &u, []behavior.Option{behavior.WithKinds(behavior.InputOnly)})

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
	got, err := eng.Translate(context.Background(), v)
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
	// Nil map, yet schema-walk resolves the value type and emits the nested path.
	got := projection(t, doc{}, behavior.DefaultResponseKinds)
	require.Equal(t, bson.M{"labels.secret": 0}, got)
}

func TestProjectionTranslatorNestedCollections(t *testing.T) {
	t.Parallel()

	type doc struct {
		Grid  [][]address          `bson:"grid"`
		ByKey map[string][]address `bson:"by_key"`
	}

	// Zero value as type carrier: schema-walk unwraps the nested collection
	// layers down to the struct element and emits an unindexed path.
	got := projection(t, doc{}, behavior.DefaultResponseKinds)
	want := bson.M{"grid.secret": 0, "by_key.secret": 0}
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
	require.Equal(t, bson.M{"labels": bson.M{"a": bson.M{"city": "NYC", "secret": "s"}}}, got)
}

func TestUpdateTranslatorStripsInsideNestedValues(t *testing.T) {
	t.Parallel()

	type item struct {
		Name    string `bson:"name"`
		Created string `bson:"created" behavior:"output_only"`
	}
	type doc struct {
		ByKey map[string]item  `bson:"by_key"`
		ByPtr map[string]*item `bson:"by_ptr"`
		Items []item           `bson:"items"`
	}

	d := doc{
		ByKey: map[string]item{"a": {Name: "n", Created: "c"}},
		ByPtr: map[string]*item{"b": {Name: "n", Created: "c"}},
		Items: []item{{Name: "n", Created: "c"}},
	}
	got := update(t, &d, []behavior.Option{behavior.WithKinds(behavior.DefaultUpdateKinds...)})
	set := got["$set"].(bson.M)

	// Map values (plain and pointer) and slice elements all drop the
	// output_only field — strip semantics are uniform at every nesting level.
	require.Equal(t, bson.M{"a": bson.M{"name": "n"}}, set["by_key"])
	require.Equal(t, bson.M{"b": bson.M{"name": "n"}}, set["by_ptr"])
	require.Equal(t, bson.A{bson.M{"name": "n"}}, set["items"])
}

func TestMapDollarKeyRejected(t *testing.T) {
	t.Parallel()

	type doc struct {
		Labels map[string]any `bson:"labels"`
	}
	eng := behavior.New[bson.M](mongo.NewInsertTranslator())
	_, err := eng.Translate(context.Background(), &doc{Labels: map[string]any{"$set": 1}})
	require.Error(t, err)
	require.ErrorContains(t, err, "$")
}

func TestMapNonStringKeyRejected(t *testing.T) {
	t.Parallel()

	type doc struct {
		M map[int]string `bson:"m"`
	}
	eng := behavior.New[bson.M](mongo.NewInsertTranslator())
	_, err := eng.Translate(context.Background(), &doc{M: map[int]string{1: "a"}})
	require.Error(t, err)
	require.ErrorContains(t, err, "map key must be string")
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
		behavior.WithTagName("beh")).Translate(context.Background(), &d)
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
	_, err := eng.Translate(context.Background(), nil)
	require.Error(t, err)
}
