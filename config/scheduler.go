// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Scheduler configuration.
const (
	defaultSchedulerTickInterval              = time.Second
	defaultSchedulerHistoryRetention          = 168 * time.Hour // 7 days
	defaultSchedulerReservedHighPrioritySlots = 2
	defaultSchedulerStaleTaskTimeout          = 30 * time.Minute
	defaultSchedulerStorageType               = SchedulerStorageTypeMemory
	defaultSchedulerMongoTasksCollection      = "scheduler_tasks"
	defaultSchedulerMongoHistoryCollection    = "scheduler_history"
	defaultSchedulerRedisKeyPrefix            = "scheduler"
	defaultSchedulerRedisMaxHistoryPerTask    = 1000
	defaultSchedulerMemoryMaxHistoryPerTask   = 1000
)

// Deprecated: Use [ConcurrencyStrategy] constants directly.
// These aliases are kept for backward compatibility.
type SchedulerConcurrencyStrategy = ConcurrencyStrategy

const (
	SchedulerConcurrencyStatic      = ConcurrencyStatic
	SchedulerConcurrencyEnvironment = ConcurrencyEnvironment
	SchedulerConcurrencyMemoryAware = ConcurrencyMemoryAware
	SchedulerConcurrencyAdaptive    = ConcurrencyAdaptive
)

// Deprecated: Use [MemoryAwareConcurrencyConfig] directly.
type SchedulerMemoryAwareConcurrencyConfig = MemoryAwareConcurrencyConfig

// Deprecated: Use [AdaptiveConcurrencyConfig] directly.
type SchedulerAdaptiveConcurrencyConfig = AdaptiveConcurrencyConfig

// SchedulerConcurrencyConfig configures the concurrency behavior of the scheduler.
// It embeds the base [ConcurrencyConfig] and adds scheduler-specific fields.
//
// Example:
//
//	concurrency := &config.SchedulerConcurrencyConfig{
//		ConcurrencyConfig: config.ConcurrencyConfig{
//			Strategy: config.ConcurrencyStatic,
//			MaxTasks: 10,
//		},
//		ReservedHighPrioritySlots: 2,
//	}
type SchedulerConcurrencyConfig struct {
	ConcurrencyConfig `yaml:",inline"`

	// ReservedHighPrioritySlots is the number of concurrency slots reserved for
	// high priority tasks. These slots cannot be used by normal/low priority tasks.
	// Applies to all strategies. Defaults to 2.
	ReservedHighPrioritySlots int `yaml:"reservedHighPrioritySlots" default:"2"`
}

// DefaultSchedulerConcurrencyConfig returns a SchedulerConcurrencyConfig with default values.
func DefaultSchedulerConcurrencyConfig() SchedulerConcurrencyConfig {
	return SchedulerConcurrencyConfig{
		ConcurrencyConfig:         DefaultConcurrencyConfig(),
		ReservedHighPrioritySlots: defaultSchedulerReservedHighPrioritySlots,
	}
}

// Validate checks that the concurrency configuration is valid.
func (c *SchedulerConcurrencyConfig) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.ConcurrencyConfig),
		validation.Field(&c.ReservedHighPrioritySlots, validation.Min(0)),
	)
}

// SchedulerStorageType defines the storage backend type for the scheduler.
type SchedulerStorageType string

const (
	// SchedulerStorageTypeMongodb represents MongoDB storage backend.
	SchedulerStorageTypeMongodb SchedulerStorageType = "mongodb"
	// SchedulerStorageTypeRedis represents Redis storage backend.
	SchedulerStorageTypeRedis SchedulerStorageType = "redis"
	// SchedulerStorageTypeMemory represents in-memory storage backend.
	SchedulerStorageTypeMemory SchedulerStorageType = "memory"
)

// SchedulerStorageMongoConfig contains MongoDB-specific settings for the scheduler storage.
type SchedulerStorageMongoConfig struct {
	// TasksCollection is the MongoDB collection name for task states.
	// Defaults to "scheduler_tasks" if not specified.
	TasksCollection string `yaml:"tasksCollection" default:"scheduler_tasks"`

	// HistoryCollection is the MongoDB collection name for task history.
	// Defaults to "scheduler_history" if not specified.
	HistoryCollection string `yaml:"historyCollection" default:"scheduler_history"`
}

// DefaultSchedulerStorageMongoConfig returns a SchedulerStorageMongoConfig with default values.
func DefaultSchedulerStorageMongoConfig() SchedulerStorageMongoConfig {
	return SchedulerStorageMongoConfig{
		TasksCollection:   defaultSchedulerMongoTasksCollection,
		HistoryCollection: defaultSchedulerMongoHistoryCollection,
	}
}

// SchedulerStorageRedisConfig contains Redis-specific settings for the scheduler storage.
type SchedulerStorageRedisConfig struct {
	// KeyPrefix is the Redis key prefix for scheduler data.
	// Defaults to "scheduler" if not specified.
	KeyPrefix string `yaml:"keyPrefix" default:"scheduler"`

	// HistoryTTL is the TTL for history entries in Redis.
	// Set to 0 to disable TTL (history cleaned by CleanupHistory only).
	// Defaults to 0 (disabled).
	HistoryTTL time.Duration `yaml:"historyTtl" default:"-"`

	// MaxHistoryPerTask is the maximum number of history entries kept per task.
	// Defaults to 1000 if not specified.
	MaxHistoryPerTask int `yaml:"maxHistoryPerTask" default:"1000"`
}

// DefaultSchedulerStorageRedisConfig returns a SchedulerStorageRedisConfig with default values.
func DefaultSchedulerStorageRedisConfig() SchedulerStorageRedisConfig {
	return SchedulerStorageRedisConfig{
		KeyPrefix:         defaultSchedulerRedisKeyPrefix,
		MaxHistoryPerTask: defaultSchedulerRedisMaxHistoryPerTask,
	}
}

// SchedulerStorageMemoryConfig contains in-memory storage settings for the scheduler.
type SchedulerStorageMemoryConfig struct {
	// MaxHistoryPerTask is the maximum number of history entries kept per task.
	// Defaults to 1000 if not specified.
	MaxHistoryPerTask int `yaml:"maxHistoryPerTask" default:"1000"`
}

// DefaultSchedulerStorageMemoryConfig returns a SchedulerStorageMemoryConfig with default values.
func DefaultSchedulerStorageMemoryConfig() SchedulerStorageMemoryConfig {
	return SchedulerStorageMemoryConfig{
		MaxHistoryPerTask: defaultSchedulerMemoryMaxHistoryPerTask,
	}
}

var schedulerStorageAllowedTypes = []SchedulerStorageType{
	SchedulerStorageTypeMongodb,
	SchedulerStorageTypeRedis,
	SchedulerStorageTypeMemory,
}

// SchedulerStorageConfig configures the scheduler storage backend.
//
// Example:
//
//	storage := &config.SchedulerStorageConfig{
//		Type: config.SchedulerStorageTypeMongodb,
//		Mongodb: &config.SchedulerStorageMongoConfig{
//			TasksCollection:   "my_tasks",
//			HistoryCollection: "my_history",
//		},
//	}
type SchedulerStorageConfig struct {
	// Type defines the storage backend type.
	// Must be one of: mongodb, redis, memory.
	// Defaults to "memory" if not specified.
	Type SchedulerStorageType `yaml:"type" default:"memory"`

	// Mongodb defines MongoDB-specific configuration.
	// Required when Type is "mongodb", ignored otherwise.
	Mongodb *SchedulerStorageMongoConfig `yaml:"mongodb" default:"-"`

	// Redis defines Redis-specific configuration.
	// Required when Type is "redis", ignored otherwise.
	Redis *SchedulerStorageRedisConfig `yaml:"redis" default:"-"`

	// Memory defines in-memory storage configuration.
	// Optional when Type is "memory", ignored otherwise.
	Memory *SchedulerStorageMemoryConfig `yaml:"memory" default:"-"`
}

func (c *SchedulerStorageConfig) storageCases() []storageCase[SchedulerStorageType] {
	return []storageCase[SchedulerStorageType]{
		{when: SchedulerStorageTypeMongodb, field: &c.Mongodb},
		{when: SchedulerStorageTypeRedis, field: &c.Redis},
		{when: SchedulerStorageTypeMemory, field: &c.Memory},
	}
}

// DefaultSchedulerStorageConfig returns a SchedulerStorageConfig with default values.
func DefaultSchedulerStorageConfig() SchedulerStorageConfig {
	return SchedulerStorageConfig{
		Type: defaultSchedulerStorageType,
	}
}

// Validate performs validation of the scheduler storage configuration.
// Ensures type is valid and corresponding configuration is provided when required.
// Returns an error if validation fails, nil otherwise.
func (c *SchedulerStorageConfig) Validate() error {
	return validateStorageConfig(c, &c.Type, schedulerStorageAllowedTypes, c.storageCases())
}

// Scheduler configures the task scheduler service.
// Controls scheduler behavior including tick interval, history retention, and concurrency.
//
// Example:
//
//	sched := &config.Scheduler{
//		TickInterval:     time.Second,
//		HistoryRetention: 7 * 24 * time.Hour,
//		Concurrency: config.SchedulerConcurrencyConfig{
//			Strategy: config.SchedulerConcurrencyStatic,
//			MaxTasks: 10,
//			ReservedHighPrioritySlots: 2,
//		},
//		Storage: &config.SchedulerStorageConfig{
//			Type: config.SchedulerStorageTypeMongodb,
//		},
//	}
type Scheduler struct {
	// TickInterval is the interval for the scheduler loop to check for tasks.
	// Defaults to 1 second if not specified.
	TickInterval time.Duration `yaml:"tickInterval" default:"1s"`

	// HistoryRetention is the duration to keep task history before cleanup.
	// Defaults to 7 days (168h) if not specified.
	HistoryRetention time.Duration `yaml:"historyRetention" default:"168h"`

	// Concurrency configures the concurrency behavior of the scheduler.
	Concurrency SchedulerConcurrencyConfig `yaml:"concurrency"`

	// StaleTaskTimeout is the duration after which a task stuck in Running status
	// is considered stale and will be reset to Active. This handles recovery from
	// process crashes where tasks were left in Running state.
	// Defaults to 30 minutes if not specified.
	StaleTaskTimeout time.Duration `yaml:"staleTaskTimeout" default:"30m"`

	// Storage defines the storage backend configuration.
	// If nil, defaults to in-memory storage.
	Storage *SchedulerStorageConfig `yaml:"storage" default:"-"`
}

// DefaultScheduler returns a Scheduler configuration with default values.
func DefaultScheduler() Scheduler {
	return Scheduler{
		TickInterval:     defaultSchedulerTickInterval,
		HistoryRetention: defaultSchedulerHistoryRetention,
		Concurrency:      DefaultSchedulerConcurrencyConfig(),
		StaleTaskTimeout: defaultSchedulerStaleTaskTimeout,
	}
}

// Validate performs validation of the scheduler configuration.
// Ensures all fields are correctly configured.
// Returns an error if validation fails, nil otherwise.
func (c *Scheduler) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.TickInterval, ozzo_rules.Duration(), validation.Min(time.Millisecond)),
		validation.Field(&c.HistoryRetention, ozzo_rules.Duration(), validation.Min(time.Second)),
		validation.Field(&c.Concurrency),
		validation.Field(&c.StaleTaskTimeout, ozzo_rules.Duration(), validation.Min(0)),
		validation.Field(&c.Storage),
	)
}
