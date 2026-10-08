// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func BenchmarkKeysetPosition(b *testing.B) {
	keys := []keysetKey{{"rank", true}, {"_id", false}}
	doc, err := bson.Marshal(bson.D{{Key: "_id", Value: bson.NewObjectID()}, {Key: "rank", Value: 42}})
	if err != nil {
		b.Fatal(err)
	}
	filter := bson.D{{Key: "group", Value: "a"}}
	b.ReportAllocs()
	for b.Loop() {
		payload, err := encodeKeysetPosition(doc, keys)
		if err != nil {
			b.Fatal(err)
		}
		values, err := decodeKeysetPosition(payload, len(keys))
		if err != nil {
			b.Fatal(err)
		}
		_ = withKeysetSeek(filter, keys, values)
	}
}
