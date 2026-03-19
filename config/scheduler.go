// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Scheduler configuration.
const (
	defaultSchedulerTickInterval              = time.Second
	defaultSchedulerHistoryRetention          = 168 * time.Hour // 7 days
	defaultSchedulerMaxConcurrentTasks        = 0
	defaultSchedulerReservedHighPrioritySlots = 2
	defaultSchedulerConcurrencyStrategy       = SchedulerConcurrencyStatic
	defaultSchedulerConcurrencyEnvironment    = concurrency.EnvironmentIOBound
	defaultSchedulerStaleTaskTimeout          = 30 * time.Minute
	defaultSchedulerStorageType               = SchedulerStorageTypeMemory
	defaultSchedulerMongoTasksCollection      = "scheduler_tasks"
	defaultSchedulerMongoHistoryCollection    = "scheduler_history"
	defaultSchedulerRedisKeyPrefix            = "scheduler"
	defaultSchedulerRedisMaxHistoryPerTask    = 1000
	defaultSchedulerMemoryMaxHistoryPerTask   = 1000
)

// SchedulerConcurrencyStrategy defines the concurrency strategy for the scheduler.
type SchedulerConcurrencyStrategy string

const (
	// SchedulerConcurrencyStatic uses a fixed concurrency limit from MaxTasks.
	SchedulerConcurrencyStatic SchedulerConcurrencyStrategy = "static"
	// SchedulerConcurrencyEnvironment selects a preset based on the environment type.
	SchedulerConcurrencyEnvironment SchedulerConcurrencyStrategy = "environment"
	// SchedulerConcurrencyMemoryAware adapts concurrency based on available memory.
	SchedulerConcurrencyMemoryAware SchedulerConcurrencyStrategy = "memory-aware"
	// SchedulerConcurrencyAdaptive combines memory and load factors for concurrency.
	SchedulerConcurrencyAdaptive SchedulerConcurrencyStrategy = "adaptive"
)

var schedulerConcurrencyAllowedStrategies = []SchedulerConcurrencyStrategy{
	SchedulerConcurrencyStatic,
	SchedulerConcurrencyEnvironment,
	SchedulerConcurrencyMemoryAware,
	SchedulerConcurrencyAdaptive,
}

var schedulerConcurrencyAllowedEnvironments = []concurrency.Environment{
	concurrency.EnvironmentMemoryConstrained,
	concurrency.EnvironmentCPUBound,
	concurrency.EnvironmentIOBound,
	concurrency.EnvironmentHighThroughput,
	concurrency.EnvironmentRateLimited,
}

// SchedulerMemoryAwareConcurrencyConfig configures the memory-aware concurrency strategy.
type SchedulerMemoryAwareConcurrencyConfig struct {
	// LowMemoryMB is the memory threshold (in MB) below which concurrency is set to 1.
	LowMemoryMB uint64 `yaml:"lowMemoryMB"`
	// MediumMemoryMB is the memory threshold (in MB) below which concurrency is conservative.
	MediumMemoryMB uint64 `yaml:"mediumMemoryMB"`
	// HighMemoryMB is the memory threshold (in MB) above which concurrency is aggressive.
	HighMemoryMB uint64 `yaml:"highMemoryMB"`
}

// Validate checks that memory thresholds are positive and ordered low < medium < high.
func (c *SchedulerMemoryAwareConcurrencyConfig) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.LowMemoryMB, validation.Required, validation.Min(uint64(1))),
		validation.Field(&c.MediumMemoryMB, validation.Required, validation.Min(uint64(1))),
		validation.Field(&c.HighMemoryMB, validation.Required, validation.Min(uint64(1))),
	)
}

// SchedulerAdaptiveConcurrencyConfig configures the adaptive concurrency strategy.
type SchedulerAdaptiveConcurrencyConfig struct {
	// MemoryLowThresholdMB is the available-memory threshold (in MB) below which
	// concurrency is reduced to 25% of the base value.
	MemoryLowThresholdMB uint64 `yaml:"memoryLowThresholdMB"`
	// MemoryMediumThresholdMB is the available-memory threshold (in MB) below which
	// concurrency is reduced to 50% of the base value.
	MemoryMediumThresholdMB uint64 `yaml:"memoryMediumThresholdMB"`
	// HighLoadThreshold is the system load above which concurrency is scaled down.
	HighLoadThreshold float64 `yaml:"highLoadThreshold"`
}

// Validate checks that the adaptive concurrency thresholds are valid.
func (c *SchedulerAdaptiveConcurrencyConfig) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.MemoryLowThresholdMB, validation.Required, validation.Min(uint64(1))),
		validation.Field(&c.MemoryMediumThresholdMB, validation.Required, validation.Min(uint64(1))),
	)
}

// SchedulerConcurrencyConfig configures the concurrency behavior of the scheduler.
//
// Example:
//
//	concurrency := &config.SchedulerConcurrencyConfig{
//		Strategy: config.SchedulerConcurrencyStatic,
//		MaxTasks: 10,
//		ReservedHighPrioritySlots: 2,
//	}
type SchedulerConcurrencyConfig struct {
	// Strategy selects the concurrency strategy.
	// Must be one of: static, environment, memory-aware, adaptive.
	// Defaults to "static".
	Strategy SchedulerConcurrencyStrategy `yaml:"strategy" default:"static"`

	// MaxTasks is the maximum number of tasks that can run concurrently.
	// Zero means unlimited. Used with the "static" strategy.
	// Defaults to 0 (unlimited).
	MaxTasks int `yaml:"maxTasks" default:"0"`

	// ReservedHighPrioritySlots is the number of concurrency slots reserved for
	// high priority tasks. These slots cannot be used by normal/low priority tasks.
	// Applies to all strategies. Defaults to 2.
	ReservedHighPrioritySlots int `yaml:"reservedHighPrioritySlots" default:"2"`

	// Environment selects a preset concurrency profile.
	// Used when Strategy is "environment".
	// Must be one of: memory-constrained, cpu-bound, io-bound, high-throughput, rate-limited.
	// Defaults to "io-bound".
	Environment concurrency.Environment `yaml:"environment" default:"io-bound"`

	// MemoryAware configures the memory-aware concurrency strategy.
	// Required when Strategy is "memory-aware".
	MemoryAware *SchedulerMemoryAwareConcurrencyConfig `yaml:"memoryAware" default:"-"`

	// Adaptive configures the adaptive concurrency strategy.
	// Required when Strategy is "adaptive".
	Adaptive *SchedulerAdaptiveConcurrencyConfig `yaml:"adaptive" default:"-"`
}

// DefaultSchedulerConcurrencyConfig returns a SchedulerConcurrencyConfig with default values.
func DefaultSchedulerConcurrencyConfig() SchedulerConcurrencyConfig {
	return SchedulerConcurrencyConfig{
		Strategy:                  defaultSchedulerConcurrencyStrategy,
		MaxTasks:                  defaultSchedulerMaxConcurrentTasks,
		ReservedHighPrioritySlots: defaultSchedulerReservedHighPrioritySlots,
		Environment:               defaultSchedulerConcurrencyEnvironment,
	}
}

// Validate checks that the concurrency configuration is valid.
func (c *SchedulerConcurrencyConfig) Validate() error {
	return ValidateStruct(c,
		validation.Field(&c.Strategy, validation.Required,
			ozzo_rules.OneOf(schedulerConcurrencyAllowedStrategies...)),
		validation.Field(&c.MaxTasks, validation.Min(0)),
		validation.Field(&c.ReservedHighPrioritySlots, validation.Min(0)),
		validation.Field(&c.Environment,
			validation.When(c.Strategy == SchedulerConcurrencyEnvironment, validation.Required,
				ozzo_rules.OneOf(schedulerConcurrencyAllowedEnvironments...))),
		validation.Field(&c.MemoryAware,
			validation.When(c.Strategy == SchedulerConcurrencyMemoryAware, validation.Required)),
		validation.Field(&c.Adaptive,
			validation.When(c.Strategy == SchedulerConcurrencyAdaptive, validation.Required)),
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
