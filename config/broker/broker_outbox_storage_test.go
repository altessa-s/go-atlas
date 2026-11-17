// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package brokerconfig_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
)

func sqlStorage(dialect string) *brokerconfig.OutboxStorageConfig {
	return &brokerconfig.OutboxStorageConfig{
		Type: brokerconfig.OutboxStorageTypeSQL,
		SQL:  &brokerconfig.OutboxSQLStorageConfig{Dialect: dialect, Table: "events_outbox"},
	}
}

func TestOutbox_StorageAndWatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		storage     *brokerconfig.OutboxStorageConfig
		watch       brokerconfig.OutboxWatch
		wantErr     bool
		unsupported bool
	}{
		{name: "mongo polling", storage: brokerconfig.DefaultOutboxStorage(), watch: brokerconfig.OutboxWatchPolling},
		{name: "mongo default watch", storage: brokerconfig.DefaultOutboxStorage()},
		{name: "mongo change stream", storage: brokerconfig.DefaultOutboxStorage(), watch: brokerconfig.OutboxWatchChangeStream},
		{name: "mongo notify", storage: brokerconfig.DefaultOutboxStorage(), watch: brokerconfig.OutboxWatchNotify, wantErr: true, unsupported: true},
		{name: "postgres polling", storage: sqlStorage(brokerconfig.SQLDialectPostgres), watch: brokerconfig.OutboxWatchPolling},
		{name: "postgres notify", storage: sqlStorage(brokerconfig.SQLDialectPostgres), watch: brokerconfig.OutboxWatchNotify},
		{
			name: "postgres change stream", storage: sqlStorage(brokerconfig.SQLDialectPostgres), watch: brokerconfig.OutboxWatchChangeStream,
			wantErr: true, unsupported: true,
		},
		{name: "mysql notify", storage: sqlStorage(brokerconfig.SQLDialectMySQL), watch: brokerconfig.OutboxWatchNotify, wantErr: true, unsupported: true},
		{name: "unknown watch", storage: brokerconfig.DefaultOutboxStorage(), watch: "push", wantErr: true, unsupported: true},
		{name: "unknown dialect", storage: sqlStorage("oracle"), wantErr: true},
		{name: "unknown type", storage: &brokerconfig.OutboxStorageConfig{Type: "redis"}, wantErr: true},
		{name: "sqldb without table", storage: sqlStorage(""), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := brokerconfig.Default().Outbox
			cfg.Storage = tc.storage
			cfg.Watch = tc.watch
			cfg.Normalize()

			err := cfg.Validate()
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			if tc.unsupported {
				require.ErrorIs(t, err, brokerconfig.ErrUnsupportedOutboxWatch)
				require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
			}
		})
	}
}

func TestOutbox_Normalize(t *testing.T) {
	t.Parallel()

	var cfg brokerconfig.Outbox
	cfg.Normalize()
	require.Equal(t, brokerconfig.DefaultOutboxStorage(), cfg.Storage)

	cfg.Storage = &brokerconfig.OutboxStorageConfig{Type: brokerconfig.OutboxStorageTypeSQL}
	cfg.Normalize()
	require.Equal(t, &brokerconfig.OutboxSQLStorageConfig{Dialect: brokerconfig.SQLDialectPostgres, Table: "events_outbox"}, cfg.Storage.SQL)

	cfg.Storage = &brokerconfig.OutboxStorageConfig{Type: brokerconfig.OutboxStorageTypeMongo}
	cfg.Normalize()
	require.Equal(t, "events_outbox", cfg.Storage.Mongo.Collection)
}

func TestOutbox_DisabledSkipsValidation(t *testing.T) {
	t.Parallel()

	cfg := brokerconfig.Outbox{Watch: brokerconfig.OutboxWatchNotify}
	require.NoError(t, cfg.Validate())
}

func TestOutbox_RequiresStorage(t *testing.T) {
	t.Parallel()

	cfg := brokerconfig.Default().Outbox
	cfg.Storage = nil
	require.Error(t, cfg.Validate())
}
