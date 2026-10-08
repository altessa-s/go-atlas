// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package bsoncodec_test

import (
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/data/mongo/bsoncodec"
)

type period struct {
	From civil.Date  `bson:"from"`
	To   *civil.Date `bson:"to"`
}

func TestCivilDateRoundTrip(t *testing.T) {
	t.Parallel()

	to := civil.Date{Year: 2026, Month: time.December, Day: 31}
	in := period{From: civil.Date{Year: 2026, Month: time.February, Day: 28}, To: &to}
	raw, err := marshal(in)
	require.NoError(t, err)
	require.Equal(t, "2026-02-28", bson.Raw(raw).Lookup("from").StringValue(), "stored as YYYY-MM-DD")
	require.Equal(t, "2026-12-31", bson.Raw(raw).Lookup("to").StringValue())

	var out period
	require.NoError(t, unmarshal(raw, &out))
	require.Equal(t, in.From, out.From)
	require.Equal(t, to, *out.To)
}

func TestCivilDateZeroAndNil(t *testing.T) {
	t.Parallel()

	raw, err := marshal(period{})
	require.NoError(t, err)
	require.Equal(t, bson.TypeNull, bson.Raw(raw).Lookup("from").Type, "the zero date is null")
	require.Equal(t, bson.TypeNull, bson.Raw(raw).Lookup("to").Type)

	var out period
	require.NoError(t, unmarshal(raw, &out))
	require.True(t, out.From.IsZero())
	require.Nil(t, out.To)
}

func TestCivilDateSortsAsStored(t *testing.T) {
	t.Parallel()

	a, err := marshal(period{From: civil.Date{Year: 999, Month: time.January, Day: 1}})
	require.NoError(t, err)
	b, err := marshal(period{From: civil.Date{Year: 2026, Month: time.January, Day: 1}})
	require.NoError(t, err)
	require.Less(t, bson.Raw(a).Lookup("from").StringValue(), bson.Raw(b).Lookup("from").StringValue(), "years are zero-padded")
}

func TestCivilDateInvalid(t *testing.T) {
	t.Parallel()

	for _, d := range []civil.Date{
		{Year: 2026, Month: time.February, Day: 30},
		{Year: 10000, Month: time.January, Day: 1},
		{Year: -1, Month: time.January, Day: 1},
	} {
		_, err := marshal(period{From: d})
		require.ErrorIs(t, err, bsoncodec.ErrInvalidDate, "%v", d)
	}

	for _, s := range []string{"2026-02-30", "2026-2-1", "26-02-01", "2026-02-01T00:00:00Z", ""} {
		raw, err := bson.Marshal(bson.M{"from": s})
		require.NoError(t, err)
		var out period
		require.ErrorIs(t, unmarshal(raw, &out), bsoncodec.ErrInvalidDate, s)
	}

	raw, err := bson.Marshal(bson.M{"from": time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)})
	require.NoError(t, err)
	var out period
	require.Error(t, unmarshal(raw, &out), "a BSON date is an instant, not a calendar date")
}
