// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Health configuration.
const (
	defaultHealthCheckInterval       = 5 * time.Second
	defaultWatcherChannelBuffer      = 10
	defaultMaxWatchersPerService     = 1000
	defaultNumShards                 = 32
	defaultMaxConcurrentHealthChecks = 10
	defaultStatusCacheTTL            = 500 * time.Millisecond
	defaultCheckTimeout              = 2 * time.Second
	defaultAdaptiveBufferThreshold   = 100
	defaultAdaptiveBufferMultiplier  = 2
	defaultMaxAdaptiveBuffer         = 100
)

// Health defines the configuration for health check coordination.
// It manages health status for multiple services, supporting caching,
// subscriptions, and adaptive buffering for high-concurrency scenarios.
type Health struct {
	// HealthCheckInterval defines how often a watcher polls health status.
	// Determines the freshness of health updates for active subscriptions.
	// Defaults to 5s.
	HealthCheckInterval time.Duration `yaml:"healthCheckInterval" default:"5s"`

	// WatcherChannelBuffer is the base buffer size for notification channels.
	// Controls how many status changes can be queued before non-blocking drops.
	// Defaults to 10.
	WatcherChannelBuffer int `yaml:"watcherChannelBuffer" default:"10"`

	// MaxWatchersPerService limits concurrent subscriptions per service.
	// Protects resources by capping the number of active watchers.
	// Value 0 means unlimited. Defaults to 1000.
	MaxWatchersPerService int `yaml:"maxWatchersPerService" default:"1000"`

	// NumShards is the number of lock shards for contention reduction.
	// Distributes activity across multiple mutexes to improve performance.
	// Defaults to 32.
	NumShards int `yaml:"numShards" default:"32"`

	// MaxConcurrentHealthChecks limits worker pool for ListStatuses.
	// Controls the parallelism of batch health check operations.
	// Defaults to 10.
	MaxConcurrentHealthChecks int `yaml:"maxConcurrentHealthChecks" default:"10"`

	// StatusCacheTTL is the cache duration for check results.
	// Reduces the load on checked services by reusing recent results.
	// Defaults to 500ms.
	StatusCacheTTL time.Duration `yaml:"statusCacheTTL" default:"500ms"`

	// CheckTimeout is the timeout per individual health check.
	// Ensures that slow or hung checks do not block the coordinator.
	// Defaults to 2s.
	CheckTimeout time.Duration `yaml:"checkTimeout" default:"2s"`

	// AdaptiveBufferThreshold is the watcher count to trigger adaptive buffering.
	// Automatically increases channel sizes when the system is under high load.
	// Defaults to 100.
	AdaptiveBufferThreshold int32 `yaml:"adaptiveBufferThreshold" default:"100"`

	// AdaptiveBufferMultiplier is the buffer multiplier under load.
	// Scale factor applied to the base buffer size when threshold is reached.
	// Defaults to 2.
	AdaptiveBufferMultiplier int `yaml:"adaptiveBufferMultiplier" default:"2"`

	// MaxAdaptiveBuffer is the maximum adaptive buffer size.
	// Cap for buffer growth to prevent excessive memory consumption.
	// Defaults to 100.
	MaxAdaptiveBuffer int `yaml:"maxAdaptiveBuffer" default:"100"`
}

// DefaultHealth returns a Health configuration with default values.
func DefaultHealth() Health {
	return Health{
		HealthCheckInterval:       defaultHealthCheckInterval,
		WatcherChannelBuffer:      defaultWatcherChannelBuffer,
		MaxWatchersPerService:     defaultMaxWatchersPerService,
		NumShards:                 defaultNumShards,
		MaxConcurrentHealthChecks: defaultMaxConcurrentHealthChecks,
		StatusCacheTTL:            defaultStatusCacheTTL,
		CheckTimeout:              defaultCheckTimeout,
		AdaptiveBufferThreshold:   defaultAdaptiveBufferThreshold,
		AdaptiveBufferMultiplier:  defaultAdaptiveBufferMultiplier,
		MaxAdaptiveBuffer:         defaultMaxAdaptiveBuffer,
	}
}

// Validate ensures the Health configuration is valid.
// It checks that intervals, timeouts and buffer sizes are within acceptable bounds.
//
// Returns an error if any validation rules fail.
func (h *Health) Validate() error {
	return ValidateStruct(h,
		validation.Field(&h.HealthCheckInterval, ozzo_rules.Duration(), validation.Min(time.Millisecond)),
		validation.Field(&h.WatcherChannelBuffer, validation.Min(0)),
		validation.Field(&h.MaxWatchersPerService, validation.Min(0)),
		validation.Field(&h.NumShards, validation.Required, validation.Min(1)),
		validation.Field(&h.MaxConcurrentHealthChecks, validation.Required, validation.Min(1)),
		validation.Field(&h.StatusCacheTTL, ozzo_rules.DurationOrZero()),
		validation.Field(&h.CheckTimeout, ozzo_rules.Duration(), validation.Min(time.Millisecond)),
		validation.Field(&h.AdaptiveBufferThreshold, validation.Min(1)),
		validation.Field(&h.AdaptiveBufferMultiplier, validation.Min(1)),
		validation.Field(&h.MaxAdaptiveBuffer, validation.Min(1)),
	)
}
