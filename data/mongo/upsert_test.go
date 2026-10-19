// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/mongo/bsoncodec"

	datamongo "github.com/altessa-s/go-atlas/data/mongo"
)

// snapshotDoc mirrors a typed document with codec-encoded fields, omitempty fields and a compound unique key.
type snapshotDoc struct {
	ID        string          `bson:"_id"`
	Source    int             `bson:"source"`
	Base      string          `bson:"base"`
	Value     decimal.Decimal `bson:"value"`
	Effective civil.Date      `bson:"effective_date"`
	Published civil.Date      `bson:"published_date,omitempty"`
	FetchedAt time.Time       `bson:"fetched_at"`
	Version   int64           `bson:"version"`
	Parts     []string        `bson:"component_ids,omitempty"`
	Note      string          `bson:"note,omitempty"`
}

// encodeGoAtlas encodes v as InsertOne on a datamongo connection would.
func encodeGoAtlas(t *testing.T, v any) bson.Raw {
	t.Helper()
	var buf bytes.Buffer
	enc := bson.NewEncoder(bson.NewDocumentWriter(&buf))
	enc.SetRegistry(bsoncodec.NewRegistry())
	require.NoError(t, enc.Encode(v))
	return buf.Bytes()
}

func TestSetOnInsertFrom(t *testing.T) {
	t.Parallel()

	doc := snapshotDoc{
		ID: "cbr:USD:RUB:2026-10-18:1", Source: 1, Base: "USD", Value: decimal.RequireFromString("81.2345"),
		Effective: civil.Date{Year: 2026, Month: time.October, Day: 18},
		FetchedAt: time.Date(2026, 10, 18, 9, 30, 0, 123_000_000, time.UTC), Version: 1, Note: "manual",
	}
	keys := []string{"source", "base", "effective_date", "version"}

	got, err := datamongo.SetOnInsertFrom(doc, keys...)
	require.NoError(t, err)

	// Same fields, order and bytes as the encoded document minus the filter keys; omitempty fields stay omitted.
	full := encodeGoAtlas(t, doc)
	elems, err := full.Elements()
	require.NoError(t, err)
	var wantKeys []string
	for _, e := range elems {
		if e.Key() == "source" || e.Key() == "base" || e.Key() == "effective_date" || e.Key() == "version" {
			continue
		}
		wantKeys = append(wantKeys, e.Key())
	}
	gotKeys := make([]string, len(got))
	for i, e := range got {
		gotKeys[i] = e.Key
		rv, ok := e.Value.(bson.RawValue)
		require.True(t, ok, "%s is a raw value", e.Key)
		require.True(t, rv.Equal(full.Lookup(e.Key)), "%s keeps its encoding", e.Key)
	}
	require.Equal(t, wantKeys, gotKeys)
	require.Equal(t, []string{"_id", "value", "fetched_at", "note"}, gotKeys)
	require.Equal(t, bson.TypeDecimal128, full.Lookup("value").Type, "the decimal codec applied")

	// The raw values survive an update document encoded by any registry.
	update, err := bson.Marshal(bson.D{{Key: "$setOnInsert", Value: got}})
	require.NoError(t, err)
	value := bson.Raw(update).Lookup("$setOnInsert", "value")
	require.Equal(t, bson.TypeDecimal128, value.Type)
	require.Equal(t, "81.2345", value.Decimal128().String())
}

func TestSetOnInsertFromEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("absent filter key is ignored", func(t *testing.T) {
		t.Parallel()
		got, err := datamongo.SetOnInsertFrom(snapshotDoc{ID: "a"}, "note", "published_date")
		require.NoError(t, err)
		require.Equal(t, "_id", got[0].Key)
		for _, e := range got {
			require.NotEqual(t, "note", e.Key)
		}
	})
	t.Run("no filter keys keeps every field", func(t *testing.T) {
		t.Parallel()
		got, err := datamongo.SetOnInsertFrom(bson.D{{Key: "a", Value: 1}, {Key: "b", Value: "x"}})
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, "a", got[0].Key)
		require.Equal(t, "b", got[1].Key)
	})
	t.Run("non-document fails", func(t *testing.T) {
		t.Parallel()
		_, err := datamongo.SetOnInsertFrom(42)
		require.Error(t, err)
	})
	t.Run("unencodable value fails", func(t *testing.T) {
		t.Parallel()
		huge := decimal.New(1, 7000) // beyond the Decimal128 exponent range
		_, err := datamongo.SetOnInsertFrom(bson.D{{Key: "v", Value: huge}})
		require.ErrorIs(t, err, bsoncodec.ErrDecimalOutOfRange)
	})
}

var errDuplicate = mongo.WriteException{WriteErrors: mongo.WriteErrors{{Code: datamongo.MongoErrorCodeDuplicateKey, Message: "E11000"}}}

func TestUpsertRetryingDuplicate(t *testing.T) {
	t.Parallel()

	errOther := errors.New("other")
	tests := []struct {
		name      string
		attempts  int
		fails     int
		failWith  error
		wantCalls int
		wantErr   func(error) bool
	}{
		{name: "success first", attempts: 2, fails: 0, failWith: errDuplicate, wantCalls: 1},
		{name: "duplicate then success", attempts: 2, fails: 1, failWith: errDuplicate, wantCalls: 2},
		{name: "duplicates exhaust two attempts", attempts: 2, fails: 10, failWith: errDuplicate, wantCalls: 2, wantErr: mongo.IsDuplicateKeyError},
		{name: "duplicates exhaust three attempts", attempts: 3, fails: 10, failWith: errDuplicate, wantCalls: 3, wantErr: mongo.IsDuplicateKeyError},
		{name: "other error not retried", attempts: 3, fails: 10, failWith: errOther, wantCalls: 1,
			wantErr: func(err error) bool { return errors.Is(err, errOther) }},
		{name: "zero attempts runs once", attempts: 0, fails: 10, failWith: errDuplicate, wantCalls: 1, wantErr: mongo.IsDuplicateKeyError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			err := datamongo.UpsertRetryingDuplicate(t.Context(), tc.attempts, func(context.Context) error {
				calls++
				if calls <= tc.fails {
					return tc.failWith
				}
				return nil
			})
			require.Equal(t, tc.wantCalls, calls)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.True(t, tc.wantErr(err), "got %v", err)
		})
	}
}

func TestStoredTime(t *testing.T) {
	t.Parallel()

	zone := time.FixedZone("MSK", 3*60*60)
	tests := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{name: "sub-millisecond dropped", in: time.Date(2026, 10, 18, 12, 0, 0, 123_456_789, time.UTC),
			want: time.Date(2026, 10, 18, 12, 0, 0, 123_000_000, time.UTC)},
		{name: "zone converted to UTC", in: time.Date(2026, 10, 18, 15, 0, 0, 999_999_999, zone),
			want: time.Date(2026, 10, 18, 12, 0, 0, 999_000_000, time.UTC)},
		{name: "before the epoch floors", in: time.Unix(0, -1).UTC(), want: time.Date(1969, 12, 31, 23, 59, 59, 999_000_000, time.UTC)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := datamongo.StoredTime(tc.in)
			require.Equal(t, tc.want, got)
			require.Equal(t, time.UTC, got.Location())

			// It is what a BSON round trip returns.
			raw, err := bson.Marshal(bson.D{{Key: "t", Value: tc.in}})
			require.NoError(t, err)
			var back struct {
				T time.Time `bson:"t"`
			}
			require.NoError(t, bson.Unmarshal(raw, &back))
			require.True(t, back.T.Equal(got), "bson %v, stored %v", back.T, got)
		})
	}
}

func TestNow(t *testing.T) {
	t.Parallel()

	before := time.Now().Add(-time.Millisecond)
	now := datamongo.Now()
	require.Equal(t, time.UTC, now.Location())
	require.Zero(t, now.Nanosecond()%int(time.Millisecond))
	require.False(t, now.Before(before))
	require.Equal(t, now, now.Round(0), "no monotonic reading")
}
