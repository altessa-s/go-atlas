// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sqlstore_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/reliability/internal/sqlstore"
)

type fakeStore struct {
	called   bool
	deadline time.Time
	hasDead  bool
	err      error
}

func (s *fakeStore) EnsureSchema(ctx context.Context) error {
	s.called = true
	s.deadline, s.hasDead = ctx.Deadline()
	return s.err
}

func TestBuild(t *testing.T) {
	t.Parallel()

	t.Run("skips schema when disabled", func(t *testing.T) {
		t.Parallel()
		st := &fakeStore{}
		got, err := sqlstore.Build(t.Context(), false, func() (*fakeStore, error) {
			return st, nil
		}, nil)
		require.NoError(t, err)
		require.Same(t, st, got)
		require.False(t, st.called)
	})

	t.Run("propagates create error without running schema", func(t *testing.T) {
		t.Parallel()
		wantErr := errors.New("create failed")
		got, err := sqlstore.Build(t.Context(), true, func() (*fakeStore, error) {
			return nil, wantErr
		}, nil)
		require.ErrorIs(t, err, wantErr)
		require.Nil(t, got)
	})

	t.Run("runs schema bounded by EnsureSchemaTimeout", func(t *testing.T) {
		t.Parallel()
		st := &fakeStore{}
		got, err := sqlstore.Build(t.Context(), true, func() (*fakeStore, error) {
			return st, nil
		}, nil)
		require.NoError(t, err)
		require.Same(t, st, got)
		require.True(t, st.called)
		require.True(t, st.hasDead)
		require.LessOrEqual(t, time.Until(st.deadline), sqlstore.EnsureSchemaTimeout)
	})

	t.Run("wraps schema error when wrapper provided", func(t *testing.T) {
		t.Parallel()
		schemaErr := errors.New("ddl lock timeout")
		st := &fakeStore{err: schemaErr}
		got, err := sqlstore.Build(t.Context(), true, func() (*fakeStore, error) {
			return st, nil
		}, func(err error) error {
			return fmt.Errorf("ensure schema: %w", err)
		})
		require.ErrorIs(t, err, schemaErr)
		require.ErrorContains(t, err, "ensure schema")
		require.Nil(t, got)
	})
}
