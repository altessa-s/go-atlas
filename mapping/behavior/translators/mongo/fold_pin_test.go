// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-atlas/mapping/behavior"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	mongo "github.com/altessa-s/go-atlas/mapping/behavior/translators/mongo"
)

// These tests pin foldObject branches the rest of the suite does not reach:
// the folded (non-driver) insert path under a stripped inline ancestor, an
// update through a non-nil inline pointer, and a layout error.

type PinFoldMeta struct {
	X int `db:"x"`
}

type PinFoldInline struct {
	Version int    `bson:"version"`
	Label   string `bson:"label"`
}

func customInsert(t *testing.T, v any, kinds ...behavior.Kind) (bson.M, error) {
	t.Helper()
	opts := coreslices.AppendIf([]behavior.Option{behavior.WithTagName("beh")}, len(kinds) > 0, behavior.WithKinds(kinds...))
	return behavior.New[bson.M](mongo.NewInsertTranslator(mongo.WithBsonTagName("db")), opts...).Translate(t.Context(), v)
}

func TestPinFoldInsertUnderStrippedInlineAncestor(t *testing.T) {
	t.Parallel()

	type doc struct {
		Meta PinFoldMeta `db:",inline" beh:"input_only"`
		Name string      `db:"name"`
	}
	got, err := customInsert(t, &doc{Meta: PinFoldMeta{X: 1}, Name: "n"}, behavior.InputOnly)
	require.NoError(t, err)
	require.Equal(t, bson.M{"x": 1, "name": "n"}, got)
}

func TestPinFoldInsertUnderStrippedNilInlinePointer(t *testing.T) {
	t.Parallel()

	type doc struct {
		*PinFoldMeta `db:",inline" beh:"input_only"`
		Name         string `db:"name"`
	}
	got, err := customInsert(t, &doc{Name: "n"}, behavior.InputOnly)
	require.NoError(t, err)
	require.Equal(t, bson.M{"name": "n"}, got)
}

func TestPinFoldUpdateThroughInlinePointer(t *testing.T) {
	t.Parallel()

	type doc struct {
		ID             string `bson:"_id"`
		*PinFoldInline `bson:",inline"`
		Name           string `bson:"name"`
	}
	got := update(t, &doc{ID: "id", PinFoldInline: &PinFoldInline{Version: 2, Label: "l"}, Name: "n"},
		[]behavior.Option{behavior.WithKinds(behavior.DefaultUpdateKinds...)})
	require.Equal(t, bson.M{"$set": bson.M{"version": 2, "label": "l", "name": "n"}}, got)
}

func TestPinFoldLayoutError(t *testing.T) {
	t.Parallel()

	type doc struct {
		A string `db:"a"`
		B string `db:"a"`
	}
	_, err := customInsert(t, &doc{A: "1", B: "2"})
	require.Error(t, err)
}
