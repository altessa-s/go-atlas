// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/requestid"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	mongostore "github.com/altessa-s/go-atlas/reliability/requestidstore/storages/mongo"
)

func TestNewRequiresDatabase(t *testing.T) {
	t.Parallel()

	_, err := mongostore.New(t.Context(), nil)
	require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
}

func TestCanceledOperations(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var s mongostore.Storage
	now := time.Now()
	_, won, err := s.Reserve(ctx, "key", requestid.Entry{}, now)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, won)
	require.ErrorIs(t, s.Complete(ctx, "key", "owner", requestid.Entry{}, now), context.Canceled)
	require.ErrorIs(t, s.Release(ctx, "key", "owner", now), context.Canceled)
}
