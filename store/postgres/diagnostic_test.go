// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestDiagnostic(t *testing.T) {
	t.Parallel()

	require.NoError(t, Diagnostic("insert widget", nil))

	cause := &pgconn.PgError{Code: "23505", Message: "duplicate key value password=secret", ConstraintName: "widgets_pkey"}
	err := Diagnostic("insert widget", cause)

	require.Equal(t, "insert widget failed (*pgconn.PgError, SQLSTATE 23505)", err.Error())
	require.NotContains(t, err.Error(), "secret")
	require.ErrorIs(t, err, cause)

	typed, ok := errors.AsType[*pgconn.PgError](err)
	require.True(t, ok)
	require.Same(t, cause, typed)

	kind, constraint := Classify(err)
	require.Equal(t, Unique, kind)
	require.Equal(t, "widgets_pkey", constraint)

	plain := Diagnostic("read widget", errors.New("dial postgres://user:secret@host"))
	require.Equal(t, "read widget failed (*errors.errorString)", plain.Error())
}
