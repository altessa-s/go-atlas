// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	datamongo "github.com/altessa-s/go-atlas/data/mongo"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestEnsureIndexesRejectsInvalidModels: invalid models are rejected before any server call (the collection is nil).
func TestEnsureIndexesRejectsInvalidModels(t *testing.T) {
	t.Parallel()

	named := func(name string, keys any) mongo.IndexModel {
		return mongo.IndexModel{Keys: keys, Options: mongoOptions.Index().SetName(name)}
	}
	tests := []struct {
		name   string
		models []mongo.IndexModel
		opts   []datamongo.IndexOption
	}{
		{name: "duplicate explicit names", models: []mongo.IndexModel{
			named("idx", bson.D{{Key: "a", Value: 1}}), named("idx", bson.D{{Key: "b", Value: 1}}),
		}},
		{name: "explicit name equals a generated one", models: []mongo.IndexModel{
			{Keys: bson.D{{Key: "a", Value: 1}}}, named("a_1", bson.D{{Key: "b", Value: 1}}),
		}},
		{name: "empty explicit name", models: []mongo.IndexModel{named("", bson.D{{Key: "a", Value: 1}})}},
		{name: "double key value", models: []mongo.IndexModel{{Keys: bson.D{{Key: "a", Value: 1.0}}}}},
		{name: "empty keys", models: []mongo.IndexModel{{Keys: bson.D{}}}},
		{name: "obsolete is a model", models: []mongo.IndexModel{named("idx", bson.D{{Key: "a", Value: 1}})},
			opts: []datamongo.IndexOption{datamongo.WithDropObsolete("idx")}},
		{name: "obsolete is a generated model name", models: []mongo.IndexModel{{Keys: bson.D{{Key: "a", Value: 1}, {Key: "b", Value: -1}}}},
			opts: []datamongo.IndexOption{datamongo.WithDropObsolete("a_1_b_-1")}},
		{name: "obsolete _id", opts: []datamongo.IndexOption{datamongo.WithDropObsolete("_id_")}},
		{name: "obsolete empty", opts: []datamongo.IndexOption{datamongo.WithDropObsolete("")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := datamongo.EnsureIndexes(t.Context(), nil, tc.models, tc.opts...)
			require.ErrorIs(t, err, datamongo.ErrInvalidIndexModels)
		})
	}
}
