// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/limiters/storages"
	"github.com/altessa-s/go-atlas/coord/limiters/storages/memory"

	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
	storagesfactory "github.com/altessa-s/go-atlas/factory/coord/limiters/storages"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

func TestBuildRegistered(t *testing.T) {
	t.Parallel()

	t.Run("registered builder is invoked", func(t *testing.T) {
		t.Parallel()
		want := memory.New()
		cfg := &limiterconfig.StorageConfig{Type: limiterconfig.StorageTypeRedis}
		builders := map[limiterconfig.StorageType]storagesfactory.StorageFunc{
			limiterconfig.StorageTypeRedis: func(ctx context.Context, got *limiterconfig.StorageConfig) (storages.Storage, error) {
				require.NotNil(t, ctx)
				require.Same(t, cfg, got)
				return want, nil
			},
		}
		got, err := storagesfactory.BuildRegistered(t.Context(), cfg, builders, "budget")
		require.NoError(t, err)
		require.Same(t, want, got)
	})

	t.Run("unregistered redis returns ErrNotRegistered", func(t *testing.T) {
		t.Parallel()
		_, err := storagesfactory.BuildRegistered(t.Context(),
			&limiterconfig.StorageConfig{Type: limiterconfig.StorageTypeRedis}, nil, "budget")
		require.ErrorIs(t, err, corefactory.ErrNotRegistered)
	})

	t.Run("unsupported nats returns ErrUnsupported", func(t *testing.T) {
		t.Parallel()
		_, err := storagesfactory.BuildRegistered(t.Context(),
			&limiterconfig.StorageConfig{Type: "nats"}, nil, "budget")
		require.ErrorIs(t, err, corefactory.ErrUnsupported)
	})
}
