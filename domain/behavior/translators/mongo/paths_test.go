// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/domain/behavior"

	mongo "github.com/altessa-s/go-atlas/domain/behavior/translators/mongo"
)

func fieldPaths(t *testing.T, v any, kinds []behavior.Kind, opts ...mongo.Option) mongo.FieldPaths {
	t.Helper()
	eng := behavior.New[mongo.FieldPaths](mongo.NewFieldPathsTranslator(opts...),
		behavior.WithKinds(kinds...), behavior.WithSchemaWalk())
	got, err := eng.Translate(t.Context(), v)
	require.NoError(t, err)
	return got
}

func TestFieldPathsTranslator(t *testing.T) {
	t.Parallel()

	got := fieldPaths(t, user{}, behavior.DefaultResponseKinds)
	require.Equal(t, mongo.FieldPaths{
		Selectable: []string{
			"_id", "active", "aliases.city", "blob", "create_time", "home.city",
			"labels", "name", "nick", "tags", "tenant_id",
		},
		Denied: []string{"aliases.secret", "home.secret", "password"},
	}, got, "parents of a denied field are not selectable, their other fields are")
}

func TestFieldPathsTranslatorMatchesProjection(t *testing.T) {
	t.Parallel()

	got := fieldPaths(t, user{}, behavior.DefaultResponseKinds)
	excl := projection(t, user{}, behavior.DefaultResponseKinds)
	require.Len(t, got.Denied, len(excl))
	for _, d := range got.Denied {
		require.Contains(t, excl, d)
	}
}

func TestFieldPathsTranslatorNothingStripped(t *testing.T) {
	t.Parallel()

	type inner struct {
		X int `bson:"x"`
	}
	type doc struct {
		Name  string `bson:"name"`
		Skip  string `bson:"-"`
		Inner inner  `bson:"inner"`
	}
	got := fieldPaths(t, doc{}, behavior.DefaultResponseKinds)
	require.Equal(t, []string{"inner", "inner.x", "name"}, got.Selectable)
	require.Empty(t, got.Denied)
}

func TestInlinePaths(t *testing.T) {
	t.Parallel()

	type meta struct {
		Secret  string `bson:"secret" behavior:"input_only"`
		Version int    `bson:"version"`
	}
	type doc struct {
		Name   string            `bson:"name"`
		Meta   meta              `bson:",inline"`
		Extras map[string]string `bson:",inline"`
	}

	got := fieldPaths(t, doc{}, behavior.DefaultResponseKinds)
	require.Equal(t, mongo.FieldPaths{Selectable: []string{"name", "version"}, Denied: []string{"secret"}}, got,
		"inlined fields live at the parent level")
	require.Equal(t, bson.M{"secret": 0}, projection(t, doc{}, behavior.DefaultResponseKinds))
}

func TestFieldPathsMaps(t *testing.T) {
	t.Parallel()

	type entry struct {
		Name   string `bson:"name"`
		Secret string `bson:"secret" behavior:"input_only"`
	}
	type clean struct {
		Name string `bson:"name"`
	}
	type doc struct {
		Entries map[string]entry   `bson:"entries"`
		Plain   map[string]clean   `bson:"plain"`
		Nested  []map[string]entry `bson:"nested"`
	}

	got := fieldPaths(t, doc{}, behavior.DefaultResponseKinds)
	require.Equal(t, mongo.FieldPaths{
		Selectable: []string{"plain"},
		Denied:     []string{"entries", "nested"},
	}, got, "map keys are dynamic: no path below a map, a map with stripped values is denied whole")
}

// Credentials is exported so it can be embedded by pointer below.
type Credentials struct {
	Secret string `bson:"secret" behavior:"input_only"`
	Login  string `bson:"login"`
}

func TestFieldPathsMapThroughNilEmbeddedPointer(t *testing.T) {
	t.Parallel()

	type entry struct {
		*Credentials `bson:",inline"`
		Name         string `bson:"name"`
	}
	type doc struct {
		Labels map[string]entry `bson:"labels"`
		Extra  map[string]entry `bson:",inline"`
	}
	type listed struct {
		Labels map[string]entry `bson:"labels"`
	}

	got := fieldPaths(t, listed{}, behavior.DefaultResponseKinds)
	require.Empty(t, got.Selectable)
	require.Equal(t, []string{"labels"}, got.Denied,
		"a secret promoted through a nil embedded pointer still denies the map")
	require.Equal(t, bson.M{"labels": 0}, projection(t, listed{}, behavior.DefaultResponseKinds))

	eng := behavior.New[mongo.FieldPaths](mongo.NewFieldPathsTranslator(),
		behavior.WithKinds(behavior.DefaultResponseKinds...), behavior.WithSchemaWalk())
	_, err := eng.Translate(t.Context(), doc{})
	require.ErrorIs(t, err, mongo.ErrStrippedInline)
}

func TestStrippedInlineMapRejected(t *testing.T) {
	t.Parallel()

	type entry struct {
		Secret string `bson:"secret" behavior:"input_only"`
	}
	type doc struct {
		Name  string           `bson:"name"`
		Extra map[string]entry `bson:",inline"`
	}
	eng := behavior.New[mongo.FieldPaths](mongo.NewFieldPathsTranslator(),
		behavior.WithKinds(behavior.DefaultResponseKinds...), behavior.WithSchemaWalk())
	_, err := eng.Translate(t.Context(), doc{})
	require.ErrorIs(t, err, mongo.ErrStrippedInline)
}

func TestStrippedInlineStructDeniesItsKeys(t *testing.T) {
	t.Parallel()

	type meta struct {
		Secret string `bson:"secret"`
		Name   string `bson:"name"`
	}
	type doc struct {
		Name string `bson:"name"`
		Meta meta   `bson:",inline" behavior:"input_only"`
	}

	got := fieldPaths(t, doc{}, behavior.DefaultResponseKinds)
	require.Equal(t, mongo.FieldPaths{Selectable: []string{"name"}, Denied: []string{"secret"}}, got,
		"the stripped inline struct's dominant keys are denied; the shadowed name belongs to the parent")
	require.Equal(t, bson.M{"secret": 0}, projection(t, doc{}, behavior.DefaultResponseKinds))
}

func TestFieldPathsShadowedInlineField(t *testing.T) {
	t.Parallel()

	type meta struct {
		Name string `bson:"name" behavior:"input_only"`
		Note string `bson:"note"`
	}
	type doc struct {
		Name string `bson:"name"`
		Meta meta   `bson:",inline"`
	}

	got := fieldPaths(t, doc{}, behavior.DefaultResponseKinds)
	require.Equal(t, mongo.FieldPaths{Selectable: []string{"name", "note"}}, got,
		"the parent's own name shadows the stripped inline one, which the codec never stores")
	require.Empty(t, projection(t, doc{}, behavior.DefaultResponseKinds))
}

func TestFieldPathsEmbeddedWithoutInline(t *testing.T) {
	t.Parallel()

	type doc struct {
		Credentials
		Name string `bson:"name"`
	}

	got := fieldPaths(t, doc{}, behavior.DefaultResponseKinds)
	require.Equal(t, mongo.FieldPaths{
		Selectable: []string{"credentials.login", "name"},
		Denied:     []string{"credentials.secret"},
	}, got, "the codec stores an embedded struct without inline as a subdocument")
}

func TestFieldPathsTranslatorCustomTag(t *testing.T) {
	t.Parallel()

	type doc struct {
		Name   string `db:"full_name"`
		Secret string `db:"secret" behavior:"input_only"`
	}
	got := fieldPaths(t, doc{}, behavior.DefaultResponseKinds, mongo.WithBsonTagName("db"))
	require.Equal(t, mongo.FieldPaths{Selectable: []string{"full_name"}, Denied: []string{"secret"}}, got)
}
