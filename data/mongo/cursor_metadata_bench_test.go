// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func BenchmarkComputeFilterHash_Empty(b *testing.B) {
	filter := bson.M{}
	for b.Loop() {
		_, _ = computeFilterHash(filter)
	}
}

func BenchmarkComputeFilterHash_Flat(b *testing.B) {
	filter := bson.M{
		"deleted_at":      nil,
		"organization_id": "f3360599-53c9-4833-9bb6-3c75e043caea",
		"status":          int64(2),
	}
	for b.Loop() {
		_, _ = computeFilterHash(filter)
	}
}

func BenchmarkComputeFilterHash_NestedAnd(b *testing.B) {
	filter := bson.M{"$and": bson.A{
		bson.M{"deleted_at": nil, "organization_id": "f3360599-53c9-4833-9bb6-3c75e043caea"},
		bson.M{"cooperation_format": int64(3)},
	}}
	for b.Loop() {
		_, _ = computeFilterHash(filter)
	}
}
