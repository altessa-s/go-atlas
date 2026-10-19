// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

func rawSpec(t *testing.T, d bson.D) bson.Raw {
	t.Helper()
	b, err := bson.Marshal(d)
	require.NoError(t, err)
	return b
}

// TestIndexMatchesModel: only a provably equal stored specification vetoes a drop.
func TestIndexMatchesModel(t *testing.T) {
	t.Parallel()

	keys := bson.D{{Key: "code", Value: 1}, {Key: "at", Value: -1}}
	base := bson.D{{Key: "v", Value: int32(2)}, {Key: "key", Value: bson.D{{Key: "code", Value: int32(1)}, {Key: "at", Value: int64(-1)}}},
		{Key: "name", Value: "idx"}}
	with := func(extra ...bson.E) bson.D { return append(append(bson.D{}, base...), extra...) }

	tests := []struct {
		name  string
		spec  bson.D
		model mongo.IndexModel
		want  bool
	}{
		{name: "plain equal, numeric types differ", spec: base, model: mongo.IndexModel{Keys: keys, Options: mongoOptions.Index().SetName("idx")}, want: true},
		{name: "unique equal", spec: with(bson.E{Key: "unique", Value: true}),
			model: mongo.IndexModel{Keys: keys, Options: mongoOptions.Index().SetName("idx").SetUnique(true)}, want: true},
		{name: "unique added", spec: base, model: mongo.IndexModel{Keys: keys, Options: mongoOptions.Index().SetName("idx").SetUnique(true)}},
		{name: "unique removed", spec: with(bson.E{Key: "unique", Value: true}), model: mongo.IndexModel{Keys: keys}},
		{name: "keys differ", spec: base, model: mongo.IndexModel{Keys: bson.D{{Key: "code", Value: 1}}}},
		{name: "key order differs", spec: base, model: mongo.IndexModel{Keys: bson.D{{Key: "at", Value: -1}, {Key: "code", Value: 1}}}},
		{name: "ttl equal", spec: with(bson.E{Key: "expireAfterSeconds", Value: int32(60)}),
			model: mongo.IndexModel{Keys: keys, Options: mongoOptions.Index().SetExpireAfterSeconds(60)}, want: true},
		{name: "ttl differs", spec: with(bson.E{Key: "expireAfterSeconds", Value: int32(60)}),
			model: mongo.IndexModel{Keys: keys, Options: mongoOptions.Index().SetExpireAfterSeconds(30)}},
		{name: "partial equal", spec: with(bson.E{Key: "partialFilterExpression", Value: bson.D{{Key: "s", Value: "x"}}}),
			model: mongo.IndexModel{Keys: keys, Options: mongoOptions.Index().SetPartialFilterExpression(bson.D{{Key: "s", Value: "x"}})}, want: true},
		{name: "partial missing", spec: with(bson.E{Key: "partialFilterExpression", Value: bson.D{{Key: "s", Value: "x"}}}),
			model: mongo.IndexModel{Keys: keys}},
		{name: "uncompared stored option", spec: with(bson.E{Key: "collation", Value: bson.D{{Key: "locale", Value: "en"}}}),
			model: mongo.IndexModel{Keys: keys}},
		{name: "uncompared model option", spec: base,
			model: mongo.IndexModel{Keys: keys, Options: mongoOptions.Index().SetCollation(&mongoOptions.Collation{Locale: "en"})}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, indexMatchesModel(rawSpec(t, tc.spec), tc.model))
		})
	}
}
