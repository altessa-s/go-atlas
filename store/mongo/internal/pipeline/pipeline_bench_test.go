// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pipeline_test

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/store/mongo/internal/pipeline"
)

func BenchmarkOffset(b *testing.B) {
	filter := rawFilter(b, bson.M{"status": "active"})
	sort := bson.D{{Key: "created_at", Value: -1}}
	for b.Loop() {
		pipeline.Offset(filter, sort, nil, 20, 40, true)
	}
}

func BenchmarkCursorResult(b *testing.B) {
	filter := rawFilter(b, bson.M{"status": "active"})
	for b.Loop() {
		pipeline.CursorResult("items", filter, true)
	}
}
