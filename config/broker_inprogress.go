// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for InProgress heartbeat manager configuration.
const (
	defaultInProgressEnabled           = true
	defaultInProgressTickSchedule      = "*/1 * * * * *" // Every second
	defaultInProgressHeartbeatInterval = 10 * time.Second
	defaultInProgressMaxEntries        = 10000
	defaultInProgressMetricsEnabled    = true
	defaultInProgressMetricsPrefix     = "inprogress"

	// maxInProgressMaxEntries is the maximum allowed value for MaxEntries.
	maxInProgressMaxEntries = 100000
)

// InProgressMetrics defines the metrics configuration for InProgress heartbeat manager.
type InProgressMetrics struct {
	// Enabled allows enabling or disabling metrics collection.
	// Defaults to true.
	Enabled bool `yaml:"enabled" default:"true"`

	// Prefix for InProgress heartbeat manager metrics.
	// Defaults to "inprogress".
	Prefix string `yaml:"prefix" default:"inprogress"`
}

// InProgress defines the configuration for InProgress heartbeat manager.
// Manages periodic heartbeat sending for long-running message handlers.
type InProgress struct {
	// Enabled determines whether the InProgress manager is enabled.
	// Defaults to true.
	Enabled bool `yaml:"enabled" default:"true"`

	// TickSchedule defines the cron schedule for the heartbeat tick cycle.
	// Uses standard cron format with seconds: "second minute hour day month weekday"
	// Defaults to "*/1 * * * * *" (every second).
	// Examples:
	//   - "*/1 * * * * *" - every second
	//   - "*/5 * * * * *" - every 5 seconds
	//   - "0 * * * * *" - every minute
	TickSchedule string `yaml:"tickSchedule" default:"*/1 * * * * *"`

	// DefaultHeartbeatInterval specifies the default interval for heartbeats.
	// Defaults to 10 seconds.
	DefaultHeartbeatInterval time.Duration `yaml:"defaultHeartbeatInterval" default:"10s"`

	// MaxEntries defines the maximum number of registered heartbeaters.
	// Must be between 1 and 100000. Setting a reasonable limit prevents
	// resource exhaustion from unbounded registration.
	// Defaults to 10000.
	MaxEntries int `yaml:"maxEntries" default:"10000"`

	// Metrics contains metrics-related settings.
	Metrics InProgressMetrics `yaml:"metrics"`
}

// Validate performs validation of the InProgress configuration.
func (c InProgress) Validate() error {
	return ValidateStructIfEnabled(c.Enabled, &c,
		validation.Field(&c.TickSchedule, validation.Required),
		validation.Field(&c.DefaultHeartbeatInterval, ozzo_rules.Duration(), validation.Min(time.Duration(1))),
		// Required is paired with Min because ozzo-validation skips every
		// rule but Required for a zero value — Min(1) alone accepts 0.
		validation.Field(&c.MaxEntries, validation.Required, validation.Min(1), validation.Max(maxInProgressMaxEntries)),
		validation.Field(&c.Metrics),
	)
}

// Validate performs validation of the InProgressMetrics configuration.
func (c InProgressMetrics) Validate() error {
	return ValidateStruct(&c,
		validation.Field(&c.Prefix, validation.When(c.Enabled, validation.Required)),
	)
}
