// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keyset_test

import (
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/store/mongo/keyset"
)

func BenchmarkFindPageBindings(b *testing.B) {
	probe := new(bindingProbe)
	q := keyset.Query{
		Filter: bson.D{{Key: "group", Value: "a"}},
		Sort:   bson.D{{Key: "rank", Value: -1}}, After: "probe",
	}
	ctx := b.Context()
	b.ReportAllocs()
	for b.Loop() {
		_, err := keyset.FindPage[bson.M](ctx, nil, probe, q)
		if !errors.Is(err, errBound) {
			b.Fatal(err)
		}
	}
}
