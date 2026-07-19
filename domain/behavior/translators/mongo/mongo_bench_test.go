// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/core/types/ptr"
	"github.com/altessa-s/go-atlas/domain/behavior"

	mongo "github.com/altessa-s/go-atlas/domain/behavior/translators/mongo"
)

func benchUser() *user {
	return &user{
		ID:         "u1",
		Name:       "Ann",
		Nick:       ptr.Wrap("a"),
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
