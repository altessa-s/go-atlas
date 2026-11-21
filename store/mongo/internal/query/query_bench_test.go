// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package query

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func BenchmarkValidateFilter(b *testing.B) {
	filter := bson.M{"$or": []bson.D{{{Key: "name", Value: "a"}}, {{Key: "name", Value: "b"}}}}
	for b.Loop() {
		if _, err := ValidateFilter(b.Context(), filter, Registry(), nil); err != nil {
			b.Fatal(err)
		}
	}
}
