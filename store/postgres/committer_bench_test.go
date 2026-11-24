// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/store/postgres"
)

func BenchmarkJoinedTransaction(b *testing.B) {
	db, _ := testhelpers.NewFakeSQL(b, nil)
	c, err := postgres.NewCommitter(db)
	require.NoError(b, err)
	require.NoError(b, c.WithTransaction(b.Context(), func(ctx context.Context) error {
		for b.Loop() {
			if err := postgres.WithTransaction(ctx, db, func(inner context.Context) error {
				_, ok := c.Detect(inner)
				if !ok {
					b.Fatal("missing transaction")
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	}))
}
