// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsoncodec_test

import (
	"bytes"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas-core/types/optional"
	"github.com/altessa-s/go-atlas/store/mongo/bsoncodec"

	atlascivil "github.com/altessa-s/go-atlas-core/time/civil"
)

func marshal(v any) ([]byte, error) {
	var out bytes.Buffer
	enc := bson.NewEncoder(bson.NewDocumentWriter(&out))
	enc.SetRegistry(bsoncodec.NewRegistry())
	err := enc.Encode(v)
	return out.Bytes(), err
}

func unmarshal(data []byte, v any) error {
	dec := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(data)))
	dec.SetRegistry(bsoncodec.NewRegistry())
	return dec.Decode(v)
}

// TestLegacyRegistryBytes compares the Atlas registry with a document encoded
// before delegation. The fixture pins both BSON types and value bytes.
func TestLegacyRegistryBytes(t *testing.T) {
	t.Parallel()
	amount := decimal.RequireFromString("12.50")
	doc := bson.D{
		{Key: "cloud", Value: civil.Date{Year: 2024, Month: time.February, Day: 29}},
		{Key: "cloudZero", Value: civil.Date{}},
		{Key: "atlas", Value: atlascivil.Date{Year: 2024, Month: time.February, Day: 29}},
		{Key: "atlasZero", Value: atlascivil.Date{}},
		{Key: "some", Value: optional.Some(amount)},
		{Key: "none", Value: optional.None[int]()},
		{Key: "ptr", Value: &amount},
		{Key: "null", Value: decimal.NullDecimal{}},
		{Key: "i32", Value: int32(42)},
		{Key: "i64", Value: int64(42)},
	}
	want, err := os.ReadFile("testdata/atlas-registry.bson")
	require.NoError(t, err)
	got, err := marshal(doc)
	require.NoError(t, err)
	require.Equal(t, want, got)
	var decoded struct {
		Some optional.Optional[decimal.Decimal]
		Ptr  *decimal.Decimal
		Null decimal.NullDecimal
		I32  decimal.Decimal
		I64  decimal.Decimal
	}
	require.NoError(t, unmarshal(want, &decoded))
	some, present := decoded.Some.Get()
	require.True(t, present)
	require.True(t, amount.Equal(some))
	require.NotNil(t, decoded.Ptr)
	require.True(t, amount.Equal(*decoded.Ptr))
	require.False(t, decoded.Null.Valid)
	require.True(t, decimal.NewFromInt(42).Equal(decoded.I32))
	require.True(t, decimal.NewFromInt(42).Equal(decoded.I64))
}
