// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsoncodec_test

import (
	"bytes"
	"github.com/altessa-s/go-atlas/core/types/optional"
	"github.com/altessa-s/go-atlas/data/mongo/bsoncodec"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"testing"
)

func BenchmarkOptionalEncode(b *testing.B) {
	var out bytes.Buffer
	enc := bson.NewEncoder(bson.NewDocumentWriter(&out))
	enc.SetRegistry(bsoncodec.NewRegistry())
	value := struct {
		Value optional.Optional[int] `bson:"value"`
	}{optional.Some(42)}
	for b.Loop() {
		out.Reset()
		require.NoError(b, enc.Encode(value))
	}
}
