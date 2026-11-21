// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongoit_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/store/mongo/indexes"
	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	mongooptions "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// TestEnsureChecksModelsBeforeDropping verifies that a malformed named model
// cannot remove an obsolete index before the create command rejects it.
func TestEnsureChecksModelsBeforeDropping(t *testing.T) {
	t.Parallel()

	coll := livebackend.MongoDatabase(t, "index_preflight").Collection("items")
	model := mongo.IndexModel{Keys: bson.D{{Key: "code", Value: 1}}, Options: mongooptions.Index().SetUnique(true)}
	name, err := coll.Indexes().CreateOne(t.Context(), model)
	require.NoError(t, err)

	invalid := mongo.IndexModel{Keys: bson.D{}, Options: mongooptions.Index().SetName("invalid")}
	err = indexes.Ensure(t.Context(), coll, []mongo.IndexModel{invalid}, indexes.WithDropObsolete(name))
	require.ErrorIs(t, err, indexes.ErrInvalidIndexModels)

	specs, err := coll.Indexes().ListSpecifications(t.Context())
	require.NoError(t, err)
	for _, spec := range specs {
		if spec.Name == name {
			require.NotNil(t, spec.Unique)
			require.True(t, *spec.Unique)
			return
		}
	}
	require.Failf(t, "obsolete index was dropped", "index %q is missing", name)
}

// TestEnsureCreatesTheValidatedModel covers a builder that cannot be evaluated a second time.
func TestEnsureCreatesTheValidatedModel(t *testing.T) {
	t.Parallel()

	coll := livebackend.MongoDatabase(t, "index_snapshot").Collection("items")
	opts := mongooptions.Index()
	calls := 0
	opts.Opts = append(opts.Opts, func(o *mongooptions.IndexOptions) error {
		calls++
		if calls > 1 {
			return errors.New("index options already evaluated")
		}
		o.Name = new("current")
		return nil
	})
	model := mongo.IndexModel{Keys: bson.D{{Key: "code", Value: 1}}, Options: opts}
	require.NoError(t, indexes.Ensure(t.Context(), coll, []mongo.IndexModel{model}, indexes.WithDropObsolete("old")))
	require.Equal(t, 1, calls)
}
