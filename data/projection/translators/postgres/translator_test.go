// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/projection"
	"github.com/altessa-s/go-atlas/data/projection/translators/postgres"
)

func TestTranslate(t *testing.T) {
	t.Parallel()

	tr, err := postgres.NewTranslator(
		projection.WithUntrustedInput(),
		projection.WithAllowedFields("name", "email", "createTime"),
		projection.WithFieldMapping(map[string]string{"createTime": "created_at"}),
		projection.WithRequiredFields("id"),
	)
	require.NoError(t, err)

	got, err := tr.Translate(projection.Spec{Paths: []string{"name", "createTime"}})
	require.NoError(t, err)
	require.Equal(t, `"created_at", "id", "name"`, got)

	_, err = tr.Translate(projection.Spec{Paths: []string{"password"}})
	require.ErrorIs(t, err, projection.ErrFieldNotAllowed)
}

func TestTranslateDefault(t *testing.T) {
	t.Parallel()

	tr, err := postgres.NewTranslator(
		projection.WithDeniedFields("password"),
		projection.WithDefaultFields("name", "email"),
		projection.WithRequiredFields("id"),
	)
	require.NoError(t, err)
	got, err := tr.Translate(projection.Spec{})
	require.NoError(t, err)
	require.Equal(t, `"email", "id", "name"`, got)
}

func TestNewTranslatorRejectsExclusionDefault(t *testing.T) {
	t.Parallel()

	_, err := postgres.NewTranslator(projection.WithDeniedFields("password"))
	require.ErrorIs(t, err, projection.ErrDefaultFieldsRequired)
}
