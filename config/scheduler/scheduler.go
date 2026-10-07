// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package schedulerconfig

import (
	"time"

	validationconfig "github.com/altessa-s/go-atlas/config/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Scheduler configuration.
const (
	defaultSchedulerTickInterval              = time.Second
	defaultSchedulerHistoryRetention          = 168 * time.Hour // 7 days
	defaultSchedulerReservedHighPrioritySlots = 2
	defaultSchedulerStaleTaskTimeout          = 30 * time.Minute
	defaultSchedulerStorageType               = StorageTypeMemory
	defaultSchedulerMongoTasksCollection      = "scheduler_tasks"
	defaultSchedulerMongoHistoryCollection    = "scheduler_history"
	defaultSchedulerRedisKeyPrefix            = "scheduler"
	defaultSchedulerRedisMaxHistoryPerTask    = 1000
	defaultSchedulerMemoryMaxHistoryPerTask   = 1000
	defaultSchedulerSQLDialect                = SQLDialectPostgres
	defaultSchedulerSQLTasksTable             = "scheduler_tasks"
	defaultSchedulerSQLHistoryTable           = "scheduler_history"
)

// MaxSchedulerInstanceIDLength bounds [Config.InstanceID]. The ID prefixes
// every run ID the scheduler persists, so it is kept short.
const MaxSchedulerInstanceIDLength = 128

// TaskConcurrency configures the concurrency behavior of the scheduler.
// It embeds the base [Concurrency] and adds scheduler-specific fields.
//
// Example:
//
//	concurrency := &schedulerconfig.TaskConcurrency{
//		Concurrency: schedulerconfig.Concurrency{
//			Strategy: schedulerconfig.ConcurrencyStatic,
//			MaxTasks: 10,
//		},
//		ReservedHighPrioritySlots: 2,
//	}
type TaskConcurrency struct {
	Concurrency `yaml:",inline"`

	// ReservedHighPrioritySlots is the number of concurrency slots reserved for
	// high priority tasks. These slots cannot be used by normal/low priority tasks.
	// Applies to all strategies. Defaults to 2.
	ReservedHighPrioritySlots int `yaml:"reservedHighPrioritySlots" default:"2"`
}

// DefaultTaskConcurrency returns a TaskConcurrency with default values.
func DefaultTaskConcurrency() TaskConcurrency {
	return TaskConcurrency{
		Concurrency:               DefaultConcurrency(),
		ReservedHighPrioritySlots: defaultSchedulerReservedHighPrioritySlots,
	}
}

// Validate checks that the concurrency configuration is valid.
func (c *TaskConcurrency) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Concurrency),
		validation.Field(&c.ReservedHighPrioritySlots, validation.Min(0)),
	)
}

// StorageType defines the storage backend type for the scheduler.
type StorageType string

const (
	// StorageTypeMongo represents MongoDB storage backend.
	StorageTypeMongo StorageType = "mongo"
	// StorageTypeRedis represents Redis storage backend.
	StorageTypeRedis StorageType = "redis"
	// StorageTypeMemory represents in-memory storage backend.
	StorageTypeMemory StorageType = "memory"
	// StorageTypeSQL represents a SQL database storage backend
	// (PostgreSQL, MySQL or MariaDB) reached through database/sql.
	StorageTypeSQL StorageType = "sqldb"
)

// SQL dialects accepted by [StorageSQLConfig.Dialect].
const (
	// SQLDialectPostgres targets PostgreSQL 12+.
	SQLDialectPostgres = "postgres"
	// SQLDialectMySQL targets MySQL 8.0+ and MariaDB 10.6+.
	SQLDialectMySQL = "mysql"
)

// StorageMongoConfig contains MongoDB-specific settings for the scheduler storage.
type StorageMongoConfig struct {
	// TasksCollection is the MongoDB collection name for task states.
	// Defaults to "scheduler_tasks" if not specified.
	TasksCollection string `yaml:"tasksCollection" default:"scheduler_tasks"`

	// HistoryCollection is the MongoDB collection name for task history.
	// Defaults to "scheduler_history" if not specified.
	HistoryCollection string `yaml:"historyCollection" default:"scheduler_history"`

	// EnsureIndexes makes the factory create the task and history indexes
	// while building the scheduler, by calling the storage's idempotent
	// EnsureIndexes. Leave it false when the indexes are provisioned
	// separately. Defaults to false.
	EnsureIndexes bool `yaml:"ensureIndexes" default:"false"`
}

// DefaultStorageMongoConfig returns a StorageMongoConfig with default values.
func DefaultStorageMongoConfig() StorageMongoConfig {
	return StorageMongoConfig{
		TasksCollection:   defaultSchedulerMongoTasksCollection,
		HistoryCollection: defaultSchedulerMongoHistoryCollection,
	}
}

// StorageRedisConfig contains Redis-specific settings for the scheduler storage.
type StorageRedisConfig struct {
	// KeyPrefix is the Redis key prefix for scheduler data.
	// Defaults to "scheduler" if not specified.
	KeyPrefix string `yaml:"keyPrefix" default:"scheduler"`

	// HistoryTTL is the TTL for history entries in Redis.
	// Set to 0 to disable TTL (history cleaned by CleanupHistory only).
	// Defaults to 0 (disabled).
	HistoryTTL time.Duration `yaml:"historyTTL"`

	// MaxHistoryPerTask is the maximum number of history entries kept per task.
	// Defaults to 1000 if not specified.
	MaxHistoryPerTask int `yaml:"maxHistoryPerTask" default:"1000"`

	// EnsureIndexes makes the factory create the RediSearch indexes (and
	// backfill task documents written by an earlier release) while building
	// the scheduler, by calling the storage's idempotent EnsureIndexes. Leave
	// it false when the indexes are provisioned separately. Defaults to false.
	EnsureIndexes bool `yaml:"ensureIndexes" default:"false"`
}

// DefaultStorageRedisConfig returns a StorageRedisConfig with default values.
func DefaultStorageRedisConfig() StorageRedisConfig {
	return StorageRedisConfig{
		KeyPrefix:         defaultSchedulerRedisKeyPrefix,
		MaxHistoryPerTask: defaultSchedulerRedisMaxHistoryPerTask,
	}
}

// StorageMemoryConfig contains in-memory storage settings for the scheduler.
type StorageMemoryConfig struct {
	// MaxHistoryPerTask is the maximum number of history entries kept per task.
	// Defaults to 1000 if not specified.
	MaxHistoryPerTask int `yaml:"maxHistoryPerTask" default:"1000"`
}

// DefaultStorageMemoryConfig returns a StorageMemoryConfig with default values.
func DefaultStorageMemoryConfig() StorageMemoryConfig {
	return StorageMemoryConfig{
		MaxHistoryPerTask: defaultSchedulerMemoryMaxHistoryPerTask,
	}
}

// StorageSQLConfig contains SQL-specific settings for the scheduler
// storage (service/scheduler/storages/sqldb). The *sql.DB itself is injected
// into the factory; the caller chooses and registers the driver.
type StorageSQLConfig struct {
	// Dialect selects the SQL flavor: "postgres" or "mysql" (MySQL 8.0+ and
	// MariaDB 10.6+). Defaults to "postgres" if not specified.
	Dialect string `yaml:"dialect" default:"postgres"`

	// TasksTable is the table name for task states, optionally schema-qualified.
	// Defaults to "scheduler_tasks" if not specified.
	TasksTable string `yaml:"tasksTable" default:"scheduler_tasks"`

	// HistoryTable is the table name for task history, optionally schema-qualified.
	// Defaults to "scheduler_history" if not specified.
	HistoryTable string `yaml:"historyTable" default:"scheduler_history"`

	// EnsureSchema makes the factory create the tables and indexes (and upgrade
	// a tasks table from an earlier release) while building the scheduler, by
	// calling the storage's idempotent EnsureSchema. Leave it false when the
	// schema is applied through migrations. Defaults to false.
	EnsureSchema bool `yaml:"ensureSchema" default:"false"`
}

// DefaultStorageSQLConfig returns a StorageSQLConfig with default values.
func DefaultStorageSQLConfig() StorageSQLConfig {
	return StorageSQLConfig{
		Dialect:      defaultSchedulerSQLDialect,
		TasksTable:   defaultSchedulerSQLTasksTable,
		HistoryTable: defaultSchedulerSQLHistoryTable,
	}
}

// Validate performs validation of the SQL scheduler storage configuration.
func (c *StorageSQLConfig) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Dialect, validation.Required, ozzo_rules.OneOf(SQLDialectPostgres, SQLDialectMySQL)),
		validation.Field(&c.TasksTable, validation.Required),
		validation.Field(&c.HistoryTable, validation.Required),
	)
}

var schedulerStorageAllowedTypes = []StorageType{
	StorageTypeMongo,
	StorageTypeRedis,
	StorageTypeMemory,
	StorageTypeSQL,
}

// StorageConfig configures the scheduler storage backend.
//
// Example:
//
//	storage := &schedulerconfig.StorageConfig{
//		Type: schedulerconfig.StorageTypeMongo,
//		Mongo: &schedulerconfig.StorageMongoConfig{
//			TasksCollection:   "my_tasks",
//			HistoryCollection: "my_history",
//		},
//	}
type StorageConfig struct {
	// Type defines the storage backend type.
	// Must be one of: mongo, redis, memory, sqldb.
	// Defaults to "memory" if not specified.
	Type StorageType `yaml:"type" default:"memory"`

	// Mongodb defines MongoDB-specific configuration.
	// Required when Type is "mongo", ignored otherwise.
	Mongo *StorageMongoConfig `yaml:"mongo" default:"-"`

	// Redis defines Redis-specific configuration.
	// Required when Type is "redis", ignored otherwise.
	Redis *StorageRedisConfig `yaml:"redis" default:"-"`

	// Memory defines in-memory storage configuration.
	// Optional when Type is "memory", ignored otherwise.
	Memory *StorageMemoryConfig `yaml:"memory" default:"-"`

	// SQL defines SQL database configuration.
	// Required when Type is "sqldb", ignored otherwise.
	SQL *StorageSQLConfig `yaml:"sqldb" default:"-"`
}

func (c *StorageConfig) storageCases() []validationconfig.StorageCase[StorageType] {
	return []validationconfig.StorageCase[StorageType]{
		{When: StorageTypeMongo, Field: &c.Mongo},
		{When: StorageTypeRedis, Field: &c.Redis},
		{When: StorageTypeMemory, Field: &c.Memory},
		{When: StorageTypeSQL, Field: &c.SQL},
	}
}

// DefaultStorageConfig returns a StorageConfig with default values.
func DefaultStorageConfig() StorageConfig {
	return StorageConfig{
		Type: defaultSchedulerStorageType,
	}
}

// Validate performs validation of the scheduler storage configuration.
// Ensures type is valid and corresponding configuration is provided when required.
// Returns an error if validation fails, nil otherwise.
func (c *StorageConfig) Validate() error {
	return validationconfig.ValidateStorage(c, &c.Type, schedulerStorageAllowedTypes, c.storageCases())
}

// Config configures the task scheduler service.
// Controls scheduler behavior including tick interval, history retention, and concurrency.
//
// Example:
//
//	sched := &schedulerconfig.Config{
//		TickInterval:     time.Second,
//		HistoryRetention: 7 * 24 * time.Hour,
//		Concurrency: schedulerconfig.TaskConcurrency{
//			Concurrency: schedulerconfig.Concurrency{
//				Strategy: schedulerconfig.ConcurrencyStatic,
//				MaxTasks: 10,
//			},
//			ReservedHighPrioritySlots: 2,
//		},
//		Storage: &schedulerconfig.StorageConfig{
//			Type: schedulerconfig.StorageTypeMongo,
//		},
//	}
type Config struct {
	// TickInterval is the interval for the scheduler loop to check for tasks.
	// Defaults to 1 second if not specified.
	TickInterval time.Duration `yaml:"tickInterval" default:"1s"`

	// HistoryRetention is the duration to keep task history before cleanup.
	// Defaults to 7 days (168h) if not specified.
	HistoryRetention time.Duration `yaml:"historyRetention" default:"168h"`

	// Concurrency configures the concurrency behavior of the scheduler.
	Concurrency TaskConcurrency `yaml:"concurrency"`

	// StaleTaskTimeout is the lease of a task run (at least 5 seconds), renewed
	// every third of it while the run executes. A run whose lease has expired —
	// its instance crashed or lost the storage — is reset to Active, which
	// handles recovery from process crashes; runs of live instances are kept.
	// Defaults to 30 minutes if not specified.
	StaleTaskTimeout time.Duration `yaml:"staleTaskTimeout" default:"30m"`

	// InstanceID identifies this scheduler instance as the owner of the task
	// runs it executes. It must be unique among the instances sharing one
	// storage. Empty (the default) means a random ID per process; a stable value
	// such as the pod name lets a restarted instance recover its own interrupted
	// runs immediately instead of waiting for their leases to expire.
	InstanceID string `yaml:"instanceID"`

	// Storage defines the storage backend configuration.
	// If nil, defaults to in-memory storage.
	Storage *StorageConfig `yaml:"storage" default:"-"`
}

// Default returns a Scheduler configuration with default values.
func Default() Config {
	return Config{
		TickInterval:     defaultSchedulerTickInterval,
		HistoryRetention: defaultSchedulerHistoryRetention,
		Concurrency:      DefaultTaskConcurrency(),
		StaleTaskTimeout: defaultSchedulerStaleTaskTimeout,
	}
}

// Validate performs validation of the scheduler configuration.
// Ensures all fields are correctly configured.
// Returns an error if validation fails, nil otherwise.
func (c *Config) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.TickInterval, ozzo_rules.Duration(), validation.Min(time.Millisecond)),
		validation.Field(&c.HistoryRetention, ozzo_rules.Duration(), validation.Min(time.Second)),
		validation.Field(&c.Concurrency),
		validation.Field(&c.StaleTaskTimeout, ozzo_rules.Duration(), validation.Min(0)),
		validation.Field(&c.InstanceID, validation.Length(0, MaxSchedulerInstanceIDLength)),
		validation.Field(&c.Storage),
	)
}
