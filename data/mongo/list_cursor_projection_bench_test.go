// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func BenchmarkValidateCursorProjection(b *testing.B) {
	projection := bson.M{"name": 1, "email": 1, "address.city": 1, "created_at": 1, "_id": 1}
	sort := bson.D{{Key: "created_at", Value: -1}}
	for b.Loop() {
		_ = validateCursorProjection(projection, "_id", sort)
	}
}
