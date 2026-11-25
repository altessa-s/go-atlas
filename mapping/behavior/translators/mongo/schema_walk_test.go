// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/mapping/behavior"

	mongo "github.com/altessa-s/go-atlas/mapping/behavior/translators/mongo"
)

// A response projection and its field policy must describe the stored schema
// even when the caller only has the zero value of the model.
func TestPathTranslatorsRequireSchemaWalk(t *testing.T) {
	t.Parallel()
	kinds := behavior.WithKinds(behavior.DefaultResponseKinds...)
	t.Run("projection", func(t *testing.T) {
		t.Parallel()
		out, err := behavior.New(mongo.NewProjectionTranslator(), kinds).Translate(t.Context(), user{})
		require.NoError(t, err)
		require.Equal(t, bson.M{"aliases.secret": 0, "home.secret": 0, "password": 0}, out)
	})
	t.Run("field paths", func(t *testing.T) {
		t.Parallel()
		out, err := behavior.New(mongo.NewFieldPathsTranslator(), kinds).Translate(t.Context(), user{})
		require.NoError(t, err)
		require.Equal(t, []string{"aliases.secret", "home.secret", "password"}, out.Denied)
		require.Contains(t, out.Selectable, "home.city")
	})
}

func TestPathTranslatorsResolveNilCollectionElements(t *testing.T) {
	t.Parallel()
	model := struct {
		Rows  []*address          `bson:"rows"`
		Named map[string]*address `bson:"named"`
	}{Rows: []*address{nil}, Named: map[string]*address{"missing": nil}}
	kinds := behavior.WithKinds(behavior.InputOnly)
	projection, err := behavior.New(mongo.NewProjectionTranslator(), kinds).Translate(t.Context(), model)
	require.NoError(t, err)
	require.Equal(t, bson.M{"rows.secret": 0, "named": 0}, projection)
	paths, err := behavior.New(mongo.NewFieldPathsTranslator(), kinds).Translate(t.Context(), model)
	require.NoError(t, err)
	require.Equal(t, mongo.FieldPaths{Selectable: []string{"rows.city"}, Denied: []string{"named", "rows.secret"}}, paths)
}
