// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func BenchmarkValidateStatelessCursor(b *testing.B) {
	sort := bson.D{{Key: "name", Value: 1}}
	filter := bson.M{"status": "active"}
	c, err := NewCursorWithMetadata(bson.NewObjectID().Hex(), sort, DefaultCursorIdField, filter, "alice")
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		if err := validateStatelessCursor(c, sort, DefaultCursorIdField, filter, true); err != nil {
			b.Fatal(err)
		}
		if err := validateCursorSortValue(c, sort, DefaultCursorIdField); err != nil {
			b.Fatal(err)
		}
	}
}
