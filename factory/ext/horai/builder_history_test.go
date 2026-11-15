// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/factory/ext/horai"

	schedulerconfig "github.com/altessa-s/go-atlas/config/scheduler"
	horai "github.com/altessa-s/go-horai"
)

var errHistory = errors.New("history storage failed")

func historyConfig(typ schedulerconfig.HistoryStorageType) *schedulerconfig.Config {
	cfg := schedulerconfig.Default()
	cfg.HistoryStorage = &schedulerconfig.HistoryStorageConfig{Type: typ}
	return &cfg
}

func TestUseHistoryStorage(t *testing.T) {
	t.Parallel()
	clickhouse := schedulerconfig.HistoryStorageTypeClickHouse

	t.Run("unregistered_fails", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(historyConfig(clickhouse)).Build(t.Context())
		require.Error(t, err)
	})

	t.Run("rejects_unknown_type", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(historyConfig("mongo")).Build(t.Context())
		require.Error(t, err)
	})

	t.Run("nil_builder_fails", func(t *testing.T) {
		t.Parallel()
		_, err := factory.New(historyConfig(clickhouse)).UseHistoryStorage(clickhouse, nil).Build(t.Context())
		require.Error(t, err)
	})

	t.Run("builder_error_propagates", func(t *testing.T) {
		t.Parallel()
		failing := func(context.Context, *schedulerconfig.Config) (horai.HistoryStore, error) { return nil, errHistory }
		_, err := factory.New(historyConfig(clickhouse)).UseHistoryStorage(clickhouse, failing).Build(t.Context())
		require.ErrorIs(t, err, errHistory)
	})
}
