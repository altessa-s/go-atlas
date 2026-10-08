// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsoncodec_test

import (
	"bytes"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/core/types/optional"
	"github.com/altessa-s/go-atlas/data/mongo/bsoncodec"
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

func BenchmarkDecimalRoundTrip(b *testing.B) {
	in := struct {
		Amount decimal.Decimal `bson:"amount"`
	}{Amount: decimal.RequireFromString("12345.6789")}
	b.ReportAllocs()
	for b.Loop() {
		raw, err := marshal(in)
		if err != nil {
			b.Fatal(err)
		}
		var out struct {
			Amount decimal.Decimal `bson:"amount"`
		}
		if err := unmarshal(raw, &out); err != nil {
			b.Fatal(err)
		}
	}
}
