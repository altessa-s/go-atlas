// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/mapping/behavior"

	mongo "github.com/altessa-s/go-atlas/mapping/behavior/translators/mongo"
)

type pinDotted struct {
	X int `bson:"a.b"`
}

func TestCollectPaths_ErrorsPin(t *testing.T) {
	t.Parallel()

	translate := func(t *testing.T, v any) error {
		t.Helper()
		eng := behavior.New[mongo.FieldPaths](mongo.NewFieldPathsTranslator(),
			behavior.WithKinds(behavior.DefaultResponseKinds...), behavior.WithSchemaWalk())
		_, err := eng.Translate(t.Context(), v)
		return err
	}

	t.Run("nested struct name", func(t *testing.T) {
		t.Parallel()
		type doc struct {
			In pinDotted `bson:"in"`
		}
		require.ErrorIs(t, translate(t, doc{}), mongo.ErrUnaddressableName)
	})

	t.Run("nested collection element name", func(t *testing.T) {
		t.Parallel()
		type doc struct {
			L [][]pinDotted `bson:"l"`
		}
		require.ErrorIs(t, translate(t, doc{}), mongo.ErrUnaddressableName)
	})

	t.Run("inline map element name", func(t *testing.T) {
		t.Parallel()
		type doc struct {
			Name  string               `bson:"name"`
			Extra map[string]pinDotted `bson:",inline"`
		}
		require.ErrorIs(t, translate(t, doc{}), mongo.ErrUnaddressableName)
	})

	t.Run("stripped inline map", func(t *testing.T) {
		t.Parallel()
		type doc struct {
			Name  string            `bson:"name"`
			Extra map[string]string `bson:",inline" behavior:"input_only"`
		}
		require.ErrorIs(t, translate(t, doc{}), mongo.ErrStrippedInline)
	})

	t.Run("duplicated key", func(t *testing.T) {
		t.Parallel()
		type doc struct {
			A int `bson:"x"`
			B int `bson:"x"`
		}
		require.ErrorContains(t, translate(t, doc{}), "duplicated key x")
	})

	t.Run("inline pointer to a non-struct", func(t *testing.T) {
		t.Parallel()
		type doc struct {
			P *int `bson:",inline"`
		}
		require.ErrorContains(t, translate(t, doc{}), "inline fields must be a struct, a struct pointer, or a map")
	})

	t.Run("inline scalar", func(t *testing.T) {
		t.Parallel()
		type doc struct {
			N int `bson:",inline"`
		}
		require.ErrorContains(t, translate(t, doc{}), "inline fields must be a struct, a struct pointer, or a map")
	})
}
