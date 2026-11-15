// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotencyconfig_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
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

	c := idempotencyconfig.Default()
	require.Equal(t, 24*time.Hour, c.TTL)
	require.Contains(t, fieldErrors(t, c.Validate()), "Storage", "storage is mandatory")

	c.Storage = &idempotencyconfig.StorageConfig{Type: idempotencyconfig.StorageTypeMemory}
	require.NoError(t, c.Validate())
}

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		mut   func(*idempotencyconfig.Config)
		field string
	}{
		{"zero ttl", func(c *idempotencyconfig.Config) { c.TTL = 0 }, "TTL"},
		{"ttl below a minute", func(c *idempotencyconfig.Config) { c.TTL = 30 * time.Second }, "TTL"},
		{"lock below a second", func(c *idempotencyconfig.Config) { c.MaxLockDuration = time.Millisecond }, "MaxLockDuration"},
		{"unknown storage", func(c *idempotencyconfig.Config) { c.Storage.Type = "etcd" }, "Storage"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := idempotencyconfig.Default()
			c.Storage = &idempotencyconfig.StorageConfig{Type: idempotencyconfig.StorageTypeRedis}
			tc.mut(&c)
			require.Contains(t, fieldErrors(t, c.Validate()), tc.field)
		})
	}

	ok := idempotencyconfig.Default()
	ok.MaxLockDuration = 5 * time.Second
	ok.Storage = &idempotencyconfig.StorageConfig{
		Type: idempotencyconfig.StorageTypeMongo, Mongo: &idempotencyconfig.MongoStorageConfig{Collection: "keys"},
	}
	require.NoError(t, ok.Validate())
}

func TestStorageConfig_Normalize(t *testing.T) {
	t.Parallel()

	for _, typ := range []idempotencyconfig.StorageType{
		idempotencyconfig.StorageTypeMemory, idempotencyconfig.StorageTypeNATS,
		idempotencyconfig.StorageTypeRedis, idempotencyconfig.StorageTypeMongo,
	} {
		c := idempotencyconfig.StorageConfig{Type: typ}
		c.Normalize()
		require.Equal(t, typ == idempotencyconfig.StorageTypeMemory, c.Memory != nil, typ)
		require.Equal(t, typ == idempotencyconfig.StorageTypeNATS, c.NATS != nil, typ)
		require.Equal(t, typ == idempotencyconfig.StorageTypeRedis, c.Redis != nil, typ)
		require.Equal(t, typ == idempotencyconfig.StorageTypeMongo, c.Mongo != nil, typ)
	}

	mongo := &idempotencyconfig.MongoStorageConfig{Collection: "c"}
	c := idempotencyconfig.StorageConfig{Type: idempotencyconfig.StorageTypeMongo, Mongo: mongo}
	c.Normalize()
	require.Same(t, mongo, c.Mongo)
}

func TestMongoStorageConfig_Validate(t *testing.T) {
	t.Parallel()

	require.NoError(t, (&idempotencyconfig.MongoStorageConfig{Collection: "c"}).Validate())
	require.Contains(t, fieldErrors(t, (&idempotencyconfig.MongoStorageConfig{}).Validate()), "Collection")

	bad := idempotencyconfig.StorageConfig{
		Type: idempotencyconfig.StorageTypeMongo, Mongo: &idempotencyconfig.MongoStorageConfig{},
	}
	require.Contains(t, fieldErrors(t, bad.Validate()), "Mongo")
}
