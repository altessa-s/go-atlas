// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxconfig_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	outboxconfig "github.com/altessa-s/go-atlas/config/outbox"
)

func sqlStorage(dialect string) *outboxconfig.Storage {
	return &outboxconfig.Storage{
		Type: outboxconfig.StorageTypeSQL,
		SQL:  &outboxconfig.SQLStorage{Dialect: dialect, Table: "events_outbox"},
	}
}

func TestStorageAndWatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		storage *outboxconfig.Storage
		watch   outboxconfig.Watch
		wantErr bool
	}{
		{name: "mongo polling", storage: outboxconfig.DefaultStorage(), watch: outboxconfig.WatchPolling},
		{name: "mongo default watch", storage: outboxconfig.DefaultStorage()},
		{name: "mongo change stream", storage: outboxconfig.DefaultStorage(), watch: outboxconfig.WatchChangeStream},
		{name: "mongo notify", storage: outboxconfig.DefaultStorage(), watch: outboxconfig.WatchNotify},
		{name: "postgres polling", storage: sqlStorage(outboxconfig.SQLDialectPostgres), watch: outboxconfig.WatchPolling},
		{name: "postgres notify", storage: sqlStorage(outboxconfig.SQLDialectPostgres), watch: outboxconfig.WatchNotify},
		{
			name: "postgres change stream", storage: sqlStorage(outboxconfig.SQLDialectPostgres), watch: outboxconfig.WatchChangeStream,
		},
		{name: "mysql notify", storage: sqlStorage(outboxconfig.SQLDialectMySQL), watch: outboxconfig.WatchNotify},
		{name: "unknown watch", storage: outboxconfig.DefaultStorage(), watch: "push", wantErr: true},
		{name: "unknown dialect", storage: sqlStorage("oracle"), wantErr: true},
		{name: "unknown type", storage: &outboxconfig.Storage{Type: "redis"}, wantErr: true},
		{name: "sqldb without table", storage: sqlStorage(""), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := outboxconfig.Default()
			cfg.Storage = tc.storage
			cfg.Watch = tc.watch
			cfg.Normalize()

			err := cfg.Validate()
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			if tc.watch == "push" {
				require.ErrorIs(t, err, outboxconfig.ErrUnsupportedWatch)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	t.Parallel()

	var cfg outboxconfig.Config
	cfg.Normalize()
	require.Equal(t, outboxconfig.DefaultStorage(), cfg.Storage)

	cfg.Storage = &outboxconfig.Storage{Type: outboxconfig.StorageTypeSQL}
	cfg.Normalize()
	require.Equal(t, &outboxconfig.SQLStorage{Dialect: outboxconfig.SQLDialectPostgres, Table: "events_outbox"}, cfg.Storage.SQL)

	cfg.Storage = &outboxconfig.Storage{Type: outboxconfig.StorageTypeMongo}
	cfg.Normalize()
	require.Equal(t, "events_outbox", cfg.Storage.Mongo.Collection)
}

func TestDisabledSkipsValidation(t *testing.T) {
	t.Parallel()

	cfg := outboxconfig.Config{Watch: outboxconfig.WatchNotify}
	require.NoError(t, cfg.Validate())
}

func TestRequiresStorage(t *testing.T) {
	t.Parallel()

	cfg := outboxconfig.Default()
	cfg.Storage = nil
	require.Error(t, cfg.Validate())
}

func TestWatchNeedsSchedule(t *testing.T) {
	t.Parallel()
	for _, watch := range []outboxconfig.Watch{outboxconfig.WatchChangeStream, outboxconfig.WatchNotify} {
		t.Run(string(watch), func(t *testing.T) {
			t.Parallel()
			cfg := outboxconfig.Default()
			cfg.Watch = watch
			if watch == outboxconfig.WatchNotify {
				cfg.Storage = sqlStorage(outboxconfig.SQLDialectPostgres)
			}
			cfg.DispatchSchedule = ""
			err := cfg.ValidateStorage()
			require.ErrorIs(t, err, outboxconfig.ErrWatchNeedsSchedule)
			require.ErrorIs(t, err, coreerrs.ErrInvalidArgument)
			cfg.DispatchSchedule = "@every 2s"
			require.NoError(t, cfg.ValidateStorage())
		})
	}
	cfg := outboxconfig.Default()
	cfg.DispatchSchedule = ""
	require.NoError(t, cfg.ValidateStorage(), "polling keeps the existing optional schedule behavior")
}

func TestSQLStorageNames(t *testing.T) {
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
			cfg := &outboxconfig.SQLStorage{Dialect: outboxconfig.SQLDialectPostgres, Table: tc.table, NotifyChannel: tc.channel}
			if tc.valid {
				require.NoError(t, cfg.Validate())
			} else {
				require.ErrorIs(t, cfg.Validate(), coreerrs.ErrInvalidArgument)
			}
		})
	}
}
