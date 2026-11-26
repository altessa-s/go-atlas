// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sagaconfig_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
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

	c := sagaconfig.Default()
	require.Equal(t, sagaconfig.StorageTypeMemory, c.Storage.Type)
	require.NotNil(t, c.Storage.Memory)
	require.Equal(t, 3, c.MaxStepAttempts)
	require.Empty(t, c.RecoveryTaskID)
	require.NoError(t, c.Validate())
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		mut   func(*sagaconfig.Config)
		field string
	}{
		{"no storage", func(c *sagaconfig.Config) { c.Storage = nil }, "Storage"},
		{"invalid storage", func(c *sagaconfig.Config) { c.Storage = &sagaconfig.StorageConfig{Type: "etcd"} }, "Storage"},
		{"zero step attempts", func(c *sagaconfig.Config) { c.MaxStepAttempts = 0 }, "MaxStepAttempts"},
		{"negative compensation attempts", func(c *sagaconfig.Config) { c.MaxCompensationAttempts = -1 }, "MaxCompensationAttempts"},
		{"negative concurrency", func(c *sagaconfig.Config) { c.StepConcurrency = -1 }, "StepConcurrency"},
		{"empty schedule", func(c *sagaconfig.Config) { c.RecoverySchedule = "" }, "RecoverySchedule"},
		{"zero batch", func(c *sagaconfig.Config) { c.RecoveryBatchSize = 0 }, "RecoveryBatchSize"},
		{"negative retention", func(c *sagaconfig.Config) { c.Retention = -time.Second }, "Retention"},
		{"empty retention schedule", func(c *sagaconfig.Config) { c.RetentionSchedule = "" }, "RetentionSchedule"},
		{"zero retention batch", func(c *sagaconfig.Config) { c.RetentionBatchSize = 0 }, "RetentionBatchSize"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := sagaconfig.Default()
			tc.mut(&c)
			require.Contains(t, fieldErrors(t, c.Validate()), tc.field)
		})
	}

	zeroTimeouts := sagaconfig.Default()
	zeroTimeouts.ExecutionTimeout = 0
	zeroTimeouts.StepTimeout = 0
	zeroTimeouts.SagaTimeout = time.Hour
	require.NoError(t, zeroTimeouts.Validate(), "zero timeouts mean disabled")
}

func TestConfig_Normalize(t *testing.T) {
	t.Parallel()

	empty := sagaconfig.Config{}
	empty.Normalize()
	require.Nil(t, empty.Storage)

	c := sagaconfig.Config{Storage: &sagaconfig.StorageConfig{Type: sagaconfig.StorageTypeSQL}}
	c.Normalize()
	require.NotNil(t, c.Storage.SQL)
}

func TestStorageConfig_Normalize(t *testing.T) {
	t.Parallel()

	for _, typ := range []sagaconfig.StorageType{
		sagaconfig.StorageTypeMemory, sagaconfig.StorageTypeNATS, sagaconfig.StorageTypeMongo,
		sagaconfig.StorageTypeRedis, sagaconfig.StorageTypeSQL,
	} {
		c := sagaconfig.StorageConfig{Type: typ}
		c.Normalize()
		require.Equal(t, typ == sagaconfig.StorageTypeMemory, c.Memory != nil, typ)
		require.Equal(t, typ == sagaconfig.StorageTypeNATS, c.NATS != nil, typ)
		require.Equal(t, typ == sagaconfig.StorageTypeMongo, c.Mongo != nil, typ)
		require.Equal(t, typ == sagaconfig.StorageTypeRedis, c.Redis != nil, typ)
		require.Equal(t, typ == sagaconfig.StorageTypeSQL, c.SQL != nil, typ)
	}

	redis := &sagaconfig.RedisStorageConfig{KeysPrefix: "p:"}
	c := sagaconfig.StorageConfig{Type: sagaconfig.StorageTypeRedis, Redis: redis}
	c.Normalize()
	require.Same(t, redis, c.Redis)
}

func TestStorageConfig_Validate(t *testing.T) {
	t.Parallel()

	valid := []*sagaconfig.StorageConfig{
		sagaconfig.DefaultStorage(),
		{Type: sagaconfig.StorageTypeNATS, NATS: &sagaconfig.NATSStorageConfig{Bucket: "saga", MaxAge: time.Hour}},
		{Type: sagaconfig.StorageTypeMongo, Mongo: &sagaconfig.MongoStorageConfig{Collection: "c"}},
		{Type: sagaconfig.StorageTypeRedis, Redis: &sagaconfig.RedisStorageConfig{KeysPrefix: "saga:", TTL: time.Hour}},
		{Type: sagaconfig.StorageTypeSQL, SQL: &sagaconfig.SQLStorageConfig{Dialect: sagaconfig.SQLDialectMySQL, Table: "t"}},
	}
	for _, c := range valid {
		require.NoError(t, c.Validate(), c.Type)
	}

	invalid := []struct {
		cfg   *sagaconfig.StorageConfig
		field string
	}{
		{&sagaconfig.StorageConfig{}, "Type"},
		{&sagaconfig.StorageConfig{Type: sagaconfig.StorageTypeNATS, NATS: &sagaconfig.NATSStorageConfig{}}, "NATS"},
		{&sagaconfig.StorageConfig{Type: sagaconfig.StorageTypeMongo, Mongo: &sagaconfig.MongoStorageConfig{}}, "Mongo"},
		{&sagaconfig.StorageConfig{Type: sagaconfig.StorageTypeRedis, Redis: &sagaconfig.RedisStorageConfig{}}, "Redis"},
		{&sagaconfig.StorageConfig{Type: sagaconfig.StorageTypeSQL, SQL: &sagaconfig.SQLStorageConfig{Dialect: "oracle"}}, "SQL"},
	}
	for _, tc := range invalid {
		require.Contains(t, fieldErrors(t, tc.cfg.Validate()), tc.field, tc.cfg.Type)
	}
}

func TestSubConfigs_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&sagaconfig.MemoryStorageConfig{}).Validate())
	require.Contains(t, fieldErrors(t, (&sagaconfig.NATSStorageConfig{}).Validate()), "Bucket")
	require.Contains(t, fieldErrors(t, (&sagaconfig.RedisStorageConfig{}).Validate()), "KeysPrefix")

	verrs := fieldErrors(t, (&sagaconfig.SQLStorageConfig{}).Validate())
	require.Contains(t, verrs, "Dialect")
	require.Contains(t, verrs, "Table")
}
