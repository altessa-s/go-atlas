// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// The resolved retention reaches the TTL index in whole seconds, rounded up so
// a resolved item is never purged before the retention has passed.
func TestTTLSeconds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   time.Duration
		want int32
	}{
		{"sub-second", time.Nanosecond, 1},
		{"whole seconds", 90 * time.Second, 90},
		{"fraction rounds up", 90*time.Second + time.Millisecond, 91},
		{"largest expiry", math.MaxInt32 * time.Second, math.MaxInt32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, ttlSeconds(tt.in))
		})
	}
}

// A retention beyond the largest TTL index expiry is refused instead of being
// capped, which would purge resolved items before the retention has passed.
func TestNewRejectsTooLongRetention(t *testing.T) {
	t.Parallel()
	for _, d := range []time.Duration{
		math.MaxInt32*time.Second + time.Nanosecond,
		time.Duration(math.MaxInt64),
	} {
		s, err := New[struct{}](new(mongo.Collection), WithResolvedRetention(d))
		require.Error(t, err, "retention %v", d)
		require.Nil(t, s)
	}
	_, err := New[struct{}](new(mongo.Collection), WithResolvedRetention(math.MaxInt32*time.Second))
	require.NoError(t, err)
}

// Move rewrites only the _id of the stored document: every other field keeps
// its position and its encoded value, whatever its type.
func TestRekeyed(t *testing.T) {
	t.Parallel()
	raw, err := bson.Marshal(bson.D{
		{Key: "state", Value: "expired"},
		{Key: "_id", Value: "old"},
		{Key: "attempts", Value: int32(3)},
		{Key: "generation", Value: int64(7)},
		{Key: "foreign", Value: bson.D{{Key: "nested", Value: bson.A{true, 1.5}}}},
	})
	require.NoError(t, err)

	doc, err := rekeyed(raw, "new")
	require.NoError(t, err)
	out, err := bson.Marshal(doc)
	require.NoError(t, err)
	want, err := bson.Marshal(bson.D{
		{Key: "state", Value: "expired"},
		{Key: "_id", Value: "new"},
		{Key: "attempts", Value: int32(3)},
		{Key: "generation", Value: int64(7)},
		{Key: "foreign", Value: bson.D{{Key: "nested", Value: bson.A{true, 1.5}}}},
	})
	require.NoError(t, err)
	require.Equal(t, bson.Raw(want), bson.Raw(out))

	_, err = rekeyed(bson.Raw{0x01}, "new")
	require.Error(t, err)
}
