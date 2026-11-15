// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keyset

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func BenchmarkPosition(b *testing.B) {
	keys := []sortKey{{"rank", true}, {"_id", false}}
	doc, err := bson.Marshal(bson.D{{Key: "_id", Value: bson.NewObjectID()}, {Key: "rank", Value: 42}})
	if err != nil {
		b.Fatal(err)
	}
	filter, err := marshalFilter(bson.D{{Key: "group", Value: "a"}}, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		payload, err := encodePosition(doc, keys)
		if err != nil {
			b.Fatal(err)
		}
		values, err := decodePosition(payload, len(keys))
		if err != nil {
			b.Fatal(err)
		}
		_ = withSeek(filter, keys, values)
	}
}
