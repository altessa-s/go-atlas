// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package indexes_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	datamongo "github.com/altessa-s/go-atlas/store/mongo/indexes"
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
		opts   []datamongo.Option
		cause  error
	}{
		{name: "unordered bson keys", models: []mongo.IndexModel{{Keys: bson.M{"a": 1, "b": -1}}}},
		{name: "unordered map before reconciliation", models: []mongo.IndexModel{
			named("valid", bson.M{"code": 1}), named("invalid", map[string]int{"a": 1, "b": -1}),
		}, opts: []datamongo.Option{datamongo.WithDropOnSpecMismatch(), datamongo.WithDropObsolete("previous")}},
		{name: "invalid model after valid model", models: []mongo.IndexModel{
			named("valid", bson.D{{Key: "a", Value: 1}}), named("invalid", bson.D{}),
		}, opts: []datamongo.Option{datamongo.WithDropObsolete("previous")}},
		{name: "obsolete is a model", models: []mongo.IndexModel{named("idx", bson.D{{Key: "a", Value: 1}})},
			opts: []datamongo.Option{datamongo.WithDropObsolete("idx")}},
		{name: "obsolete is a generated model name", models: []mongo.IndexModel{{Keys: bson.D{{Key: "a", Value: 1}, {Key: "b", Value: -1}}}},
			opts: []datamongo.Option{datamongo.WithDropObsolete("a_1_b_-1")}},
		{name: "obsolete _id", opts: []datamongo.Option{datamongo.WithDropObsolete("previous", "_id_")}},
		{name: "obsolete wildcard after valid name", opts: []datamongo.Option{datamongo.WithDropObsolete("previous", "*")}, cause: mongo.ErrMultipleIndexDrop},
		{name: "obsolete empty", opts: []datamongo.Option{datamongo.WithDropObsolete("")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := datamongo.Ensure(t.Context(), nil, tc.models, tc.opts...)
			require.ErrorIs(t, err, datamongo.ErrInvalidIndexModels)
			if tc.cause != nil {
				require.ErrorIs(t, err, tc.cause)
			}
		})
	}
}
