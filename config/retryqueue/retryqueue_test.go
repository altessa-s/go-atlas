// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package retryqueueconfig_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	retryqueueconfig "github.com/altessa-s/go-atlas/config/retryqueue"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func fieldErrors(t *testing.T, err error) validation.Errors {
	t.Helper()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	return verrs
}

func TestDefault(t *testing.T) {
	t.Parallel()
	c := retryqueueconfig.Default()
	require.Equal(t, retryqueueconfig.StorageTypeMemory, c.Storage.Type)
	require.NotNil(t, c.Storage.Memory)
	require.NoError(t, c.Validate())
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		storage *retryqueueconfig.StorageConfig
		ok      bool
	}{
		{"no storage", nil, false},
		{"unknown type", &retryqueueconfig.StorageConfig{Type: "etcd"}, false},
		{"mongo", &retryqueueconfig.StorageConfig{Type: retryqueueconfig.StorageTypeMongo, Mongo: &retryqueueconfig.MongoStorageConfig{
			Collection: "retry_items", FlagField: "flagged", FlagReasonField: "flagReason", ResolvedRetention: time.Hour,
		}}, true},
		{"mongo empty collection", &retryqueueconfig.StorageConfig{Type: retryqueueconfig.StorageTypeMongo, Mongo: &retryqueueconfig.MongoStorageConfig{
			FlagField: "flagged", FlagReasonField: "flagReason",
		}}, false},
		{"mongo empty flag field", &retryqueueconfig.StorageConfig{Type: retryqueueconfig.StorageTypeMongo, Mongo: &retryqueueconfig.MongoStorageConfig{
			Collection: "retry_items", FlagReasonField: "flagReason",
		}}, false},
		{"mongo duplicate flag names", &retryqueueconfig.StorageConfig{Type: retryqueueconfig.StorageTypeMongo, Mongo: &retryqueueconfig.MongoStorageConfig{
			Collection: "retry_items", FlagField: " statusFlag ", FlagReasonField: "statusFlag",
		}}, false},
		{"mongo reserved flag name", &retryqueueconfig.StorageConfig{Type: retryqueueconfig.StorageTypeMongo, Mongo: &retryqueueconfig.MongoStorageConfig{
			Collection: "retry_items", FlagField: "state", FlagReasonField: "flagReason",
		}}, false},
		{"mongo negative retention", &retryqueueconfig.StorageConfig{Type: retryqueueconfig.StorageTypeMongo, Mongo: &retryqueueconfig.MongoStorageConfig{
			Collection: "retry_items", FlagField: "flagged", FlagReasonField: "flagReason", ResolvedRetention: -time.Second,
		}}, false},
		{"sqldb", &retryqueueconfig.StorageConfig{Type: retryqueueconfig.StorageTypeSQL, SQL: &retryqueueconfig.SQLStorageConfig{
			Dialect: retryqueueconfig.SQLDialectMySQL, Table: "app.retry_items", EnsureSchema: true,
		}}, true},
		{"sqldb unknown dialect", &retryqueueconfig.StorageConfig{Type: retryqueueconfig.StorageTypeSQL, SQL: &retryqueueconfig.SQLStorageConfig{
			Dialect: "oracle", Table: "retry_items",
		}}, false},
		{"sqldb empty table", &retryqueueconfig.StorageConfig{Type: retryqueueconfig.StorageTypeSQL, SQL: &retryqueueconfig.SQLStorageConfig{
			Dialect: retryqueueconfig.SQLDialectPostgres,
		}}, false},
		{"sqldb invalid table", &retryqueueconfig.StorageConfig{Type: retryqueueconfig.StorageTypeSQL, SQL: &retryqueueconfig.SQLStorageConfig{
			Dialect: retryqueueconfig.SQLDialectPostgres, Table: "retry-items",
		}}, false},
		{"sqldb purge loop", &retryqueueconfig.StorageConfig{Type: retryqueueconfig.StorageTypeSQL, SQL: &retryqueueconfig.SQLStorageConfig{
			Dialect: retryqueueconfig.SQLDialectPostgres, Table: "retry_items", ResolvedRetention: time.Hour, PurgeInterval: time.Minute,
		}}, true},
		{"sqldb purge without retention", &retryqueueconfig.StorageConfig{Type: retryqueueconfig.StorageTypeSQL, SQL: &retryqueueconfig.SQLStorageConfig{
			Dialect: retryqueueconfig.SQLDialectPostgres, Table: "retry_items", PurgeInterval: time.Minute,
		}}, true},
		{"sqldb negative purge interval", &retryqueueconfig.StorageConfig{Type: retryqueueconfig.StorageTypeSQL, SQL: &retryqueueconfig.SQLStorageConfig{
			Dialect: retryqueueconfig.SQLDialectPostgres, Table: "retry_items", ResolvedRetention: time.Hour, PurgeInterval: -time.Minute,
		}}, false},
		{"sqldb negative retention", &retryqueueconfig.StorageConfig{Type: retryqueueconfig.StorageTypeSQL, SQL: &retryqueueconfig.SQLStorageConfig{
			Dialect: retryqueueconfig.SQLDialectPostgres, Table: "retry_items", ResolvedRetention: -time.Second,
		}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := retryqueueconfig.Config{Storage: tc.storage}
			err := c.Validate()
			if tc.ok {
				require.NoError(t, err)
				return
			}
			require.Contains(t, fieldErrors(t, err), "Storage")
		})
	}
}

func TestConfig_Normalize(t *testing.T) {
	t.Parallel()
	empty := retryqueueconfig.Config{}
	empty.Normalize()
	require.Nil(t, empty.Storage)

	for _, typ := range []retryqueueconfig.StorageType{
		retryqueueconfig.StorageTypeMemory, retryqueueconfig.StorageTypeMongo, retryqueueconfig.StorageTypeSQL,
	} {
		c := retryqueueconfig.Config{Storage: &retryqueueconfig.StorageConfig{Type: typ}}
		c.Normalize()
		switch typ {
		case retryqueueconfig.StorageTypeMemory:
			require.NotNil(t, c.Storage.Memory)
		case retryqueueconfig.StorageTypeMongo:
			require.NotNil(t, c.Storage.Mongo)
		case retryqueueconfig.StorageTypeSQL:
			require.NotNil(t, c.Storage.SQL)
		}
	}
}
