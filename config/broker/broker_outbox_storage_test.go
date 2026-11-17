// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package brokerconfig_test

import (
	"strings"
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

func TestOutbox_WatchNeedsSchedule(t *testing.T) {
	t.Parallel()
	for _, watch := range []brokerconfig.OutboxWatch{brokerconfig.OutboxWatchChangeStream, brokerconfig.OutboxWatchNotify} {
		t.Run(string(watch), func(t *testing.T) {
			t.Parallel()
			cfg := brokerconfig.Default().Outbox
			cfg.Watch = watch
			if watch == brokerconfig.OutboxWatchNotify {
				cfg.Storage = sqlStorage(brokerconfig.SQLDialectPostgres)
			}
			cfg.DispatchSchedule = ""
			err := cfg.ValidateStorage()
			require.ErrorIs(t, err, brokerconfig.ErrOutboxWatchNeedsSchedule)
			require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
			cfg.DispatchSchedule = "@every 2s"
			require.NoError(t, cfg.ValidateStorage())
		})
	}
	cfg := brokerconfig.Default().Outbox
	cfg.DispatchSchedule = ""
	require.NoError(t, cfg.ValidateStorage(), "polling keeps the existing optional schedule behavior")
}

func TestOutboxSQLStorageConfig_Names(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, table, channel string
		valid                bool
	}{
		{"qualified table", "app.Events", "", true},
		{"custom channel", "events", "Wake_1", true},
		{"invalid table", "bad-table", "", false},
		{"long table", strings.Repeat("t", 64), "", false},
		{"qualified channel", "events", "app.wake", false},
		{"invalid channel", "events", "bad-channel", false},
		{"long channel", "events", strings.Repeat("c", 64), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &brokerconfig.OutboxSQLStorageConfig{Dialect: brokerconfig.SQLDialectPostgres, Table: tc.table, NotifyChannel: tc.channel}
			if tc.valid {
				require.NoError(t, cfg.Validate())
			} else {
				require.ErrorIs(t, cfg.Validate(), coreerrs.ErrInvalidArgument)
			}
		})
	}
}
