// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/store/mongo"
	"github.com/altessa-s/go-atlas/store/mongo"

	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	memorystorage "github.com/altessa-s/go-atlas/store/mongo/storages/memory"
)

var errBuild = errors.New("build failed")

func TestBuild_UnregisteredDriverTypes(t *testing.T) {
	t.Parallel()

	for _, typ := range []storageconfig.BackendType{storageconfig.BackendTypeRedis, storageconfig.BackendTypeNATS} {
		t.Run(string(typ), func(t *testing.T) {
			t.Parallel()
			_, err := factory.New(&storageconfig.BackendConfig{Type: typ}).Build(t.Context())
			require.Error(t, err)
		})
	}
}

func TestUseStorage(t *testing.T) {
	t.Parallel()
	cfg := &storageconfig.BackendConfig{Type: storageconfig.BackendTypeNATS}

	_, err := factory.New(cfg).UseStorage(storageconfig.BackendTypeNATS, nil).Build(t.Context())
	require.Error(t, err)

	failing := func(context.Context, *storageconfig.BackendConfig, time.Duration, *slog.Logger) (mongo.Storage, error) {
		return nil, errBuild
	}
	_, err = factory.New(cfg).UseStorage(storageconfig.BackendTypeNATS, failing).Build(t.Context())
	require.ErrorIs(t, err, errBuild)

	want := memorystorage.New(0)
	logger := slog.New(slog.DiscardHandler)
	registered := func(_ context.Context, got *storageconfig.BackendConfig, ttl time.Duration, l *slog.Logger) (mongo.Storage, error) {
		require.Same(t, cfg, got)
		require.Equal(t, time.Hour, ttl)
		require.Same(t, logger, l)
		return want, nil
	}
	storage, err := factory.New(cfg).
		UseLogger(logger).
		UseStorage(storageconfig.BackendTypeNATS, failing).
		UseStorage(storageconfig.BackendTypeNATS, registered).
		WithTTL(time.Hour).
		Build(t.Context())
	require.NoError(t, err)
	require.Same(t, want, storage, "a later registration replaces the earlier one")
}
