// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package requestidstoreconfig

import (
	"time"

	"github.com/altessa-s/go-atlas/config/internal/validators"

	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	validationconfig "github.com/altessa-s/go-nomos/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// StorageType selects a distributed AIP-155 store.
type StorageType string

const (
	StorageTypeRedis StorageType = "redis"
	StorageTypeMongo StorageType = "mongo"
	StorageTypeSQL   StorageType = "sqldb"
	StorageTypeNATS  StorageType = "nats"
)

// Config selects the storage backend and its settings.
type Config struct {
	Type  StorageType                `yaml:"type"`
	Redis *storageconfig.RedisConfig `yaml:"redis" default:"-"`
	Mongo *MongoConfig               `yaml:"mongo" default:"-"`
	SQL   *SQLConfig                 `yaml:"sqldb" default:"-"`
	NATS  *NATSConfig                `yaml:"nats" default:"-"`
}

// MongoConfig selects the request ID collection.
type MongoConfig struct {
	Collection string `yaml:"collection" default:"request_ids"`
}

// NATSConfig selects the bucket and the interval between expiry scans.
type NATSConfig struct {
	Bucket        string        `yaml:"bucket" default:"requestid"`
	Replicas      int           `yaml:"replicas" default:"1"`
	PurgeInterval time.Duration `yaml:"purgeInterval" default:"1m"`
}

// SQLConfig selects the dialect, schema management and cleanup settings.
type SQLConfig struct {
	Dialect        string        `yaml:"dialect" default:"postgres"`
	Table          string        `yaml:"table" default:"request_id_entries"`
	EnsureSchema   bool          `yaml:"ensureSchema"`
	PurgeInterval  time.Duration `yaml:"purgeInterval" default:"1m"`
	PurgeBatchSize int           `yaml:"purgeBatchSize" default:"1000"`
}

// Normalize allocates the selected block so the configuration loader can apply defaults.
func (c *Config) Normalize() {
	switch c.Type {
	case StorageTypeRedis:
		if c.Redis == nil {
			c.Redis = new(storageconfig.RedisConfig)
		}
	case StorageTypeNATS:
		if c.NATS == nil {
			c.NATS = new(NATSConfig)
		}
	case StorageTypeMongo:
		if c.Mongo == nil {
			c.Mongo = new(MongoConfig)
		}
	case StorageTypeSQL:
		if c.SQL == nil {
			c.SQL = new(SQLConfig)
		}
	}
}

// Validate checks the selector and requires the corresponding block.
func (c *Config) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Type, validation.Required, validation.In(StorageTypeRedis, StorageTypeNATS, StorageTypeSQL, StorageTypeMongo)),
		validation.Field(&c.Redis, validation.When(c.Type == StorageTypeRedis, validation.Required)),
		validation.Field(&c.NATS, validation.When(c.Type == StorageTypeNATS, validation.Required)),
		validation.Field(&c.SQL, validation.When(c.Type == StorageTypeSQL, validation.Required)),
		validation.Field(&c.Mongo, validation.When(c.Type == StorageTypeMongo, validation.Required)))
}

// Validate checks the collection name.
func (c *MongoConfig) Validate() error {
	return validationconfig.ValidateStruct(c, validation.Field(&c.Collection, validation.Required))
}

// Validate checks the dedicated bucket and purge schedule.
func (c *NATSConfig) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Bucket, validation.Required),
		validation.Field(&c.Replicas, validation.Required, validation.Min(1), validation.Max(storageconfig.MaxNATSReplicas)),
		validation.Field(&c.PurgeInterval, validation.Min(time.Duration(0))))
}

// Validate checks the SQL dialect, table identifier and purge bounds.
func (c *SQLConfig) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Dialect, validation.Required, validation.In("postgres", "mysql")),
		validation.Field(&c.Table, validation.Required, validators.SQLTableName),
		validation.Field(&c.PurgeInterval, validation.Min(time.Duration(0))),
		validation.Field(&c.PurgeBatchSize, validation.Required, validation.Min(1), validation.Max(validators.MaxSQLKeyBatch)))
}
