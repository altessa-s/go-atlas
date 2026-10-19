// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/domain/behavior"

	mongo "github.com/altessa-s/go-atlas/domain/behavior/translators/mongo"
)

func benchUser() *user {
	return &user{
		ID:         "u1",
		Name:       "Ann",
		Nick:       new("a"),
		Active:     true,
		Tags:       []string{"a", "b", "c"},
		Blob:       []byte{0x01, 0x02, 0x03},
		CreateTime: time.Now(),
		TenantID:   "t1",
		Password:   "secret",
		Home:       &address{City: "NYC", Secret: "s"},
		Aliases:    []address{{City: "LA", Secret: "x"}, {City: "SF", Secret: "y"}},
		Labels:     map[string]string{"k1": "v1", "k2": "v2"},
	}
}

func BenchmarkInsertTranslator(b *testing.B) {
	u := benchUser()
	eng := behavior.New[bson.M](mongo.NewInsertTranslator())
	b.ReportAllocs()
	for b.Loop() {
		if _, err := eng.Translate(b.Context(), u); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUpdateTranslator(b *testing.B) {
	u := benchUser()
	eng := behavior.New[bson.M](mongo.NewUpdateTranslator(), behavior.WithKinds(behavior.DefaultUpdateKinds...))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := eng.Translate(b.Context(), u); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkProjectionTranslator(b *testing.B) {
	eng := behavior.New[bson.M](mongo.NewProjectionTranslator(),
		behavior.WithKinds(behavior.DefaultResponseKinds...), behavior.WithSchemaWalk())
	b.ReportAllocs()
	for b.Loop() {
		if _, err := eng.Translate(b.Context(), user{}); err != nil {
			b.Fatal(err)
		}
	}
}

// benchNested is an update payload whose nested objects hold protected fields
// at several depths, so the update is written by descendant paths; its nil
// pointer and empty slice hold nothing protected and are unset.
type benchNested struct {
	ID      string         `bson:"_id" behavior:"identifier"`
	Profile *nestedProfile `bson:"profile"`
	Wrap    *nestedWrap    `bson:"wrap"`
	Home    *address       `bson:"home"`
	Tags    []string       `bson:"tags"`
}

func BenchmarkUpdateTranslatorNestedProtected(b *testing.B) {
	v := &benchNested{
		ID:      "r1",
		Profile: &nestedProfile{UID: "p1", Name: "Ann", Nick: new("a"), Tags: []string{"x"}, Created: time.Now()},
		Wrap:    &nestedWrap{Note: "n", Inner: nestedProfile{UID: "p2", Name: "Bob", Created: time.Now()}},
	}
	eng := behavior.New[bson.M](mongo.NewUpdateTranslator(), behavior.WithKinds(behavior.DefaultUpdateKinds...))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := eng.Translate(b.Context(), v); err != nil {
			b.Fatal(err)
		}
	}
}

// benchCollections is an update payload replacing collections whose elements
// carry identifiers (written with each element) and nested values.
type benchCollections struct {
	ID    string                  `bson:"_id" behavior:"identifier"`
	Items []taggedIDItem          `bson:"items"`
	ByKey map[string]taggedIDItem `bson:"by_key"`
	Ptrs  []*idItem               `bson:"ptrs"`
}

func BenchmarkUpdateTranslatorCollectionReplacement(b *testing.B) {
	v := &benchCollections{
		ID:    "r1",
		Items: []taggedIDItem{{ID: "i1", Name: "a"}, {ID: "i2", Name: "b"}, {ID: "i3", Name: "c"}},
		ByKey: map[string]taggedIDItem{"k1": {ID: "m1", Name: "x"}, "k2": {ID: "m2", Name: "y"}},
		Ptrs:  []*idItem{{ID: "p1", Name: "p"}, nil},
	}
	eng := behavior.New[bson.M](mongo.NewUpdateTranslator(), behavior.WithKinds(behavior.DefaultUpdateKinds...))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := eng.Translate(b.Context(), v); err != nil {
			b.Fatal(err)
		}
	}
}
