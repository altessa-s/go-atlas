// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for NatsRecovery configuration.
const (
	defaultRecoveryEnabled              = true
	defaultRecoveryMaxAttempts          = 3
	defaultRecoveryBackoff              = 5 * time.Second
	defaultRecoveryStaleTimeout         = 20 * time.Minute
	defaultRecoveryHealthCheckSchedule  = "@every 5m"
	defaultRecoveryStaleCleanupSchedule = "@every 1m"
	maxRecoveryAttempts                 = 100
)

// NatsRecovery defines configuration for automatic NATS JetStream stream/consumer recovery.
type NatsRecovery struct {
	// Enabled determines whether recovery is enabled.
	// Defaults to true.
	Enabled bool `yaml:"enabled" default:"true"`

	// HealthCheckSchedule defines the cron schedule for health checks.
	// Acts as fallback detection when advisory events are missed.
	// Supports cron expressions and intervals (e.g., "@every 5m", "0 */5 * * * *").
	// Defaults to "@every 5m".
	HealthCheckSchedule string `yaml:"healthCheckSchedule" default:"@every 5m"`

	// MaxRecoveryAttempts defines the maximum number of recovery attempts before giving up.
	// Must be between 1 and 100.
	// Defaults to 3.
	MaxRecoveryAttempts int `yaml:"maxRecoveryAttempts" default:"3"`

	// RecoveryBackoff defines the base backoff duration between recovery attempts.
	// Actual backoff is calculated as backoff * attemptNumber.
	// Defaults to 5 seconds.
	RecoveryBackoff time.Duration `yaml:"recoveryBackoff" default:"5s"`

	// StaleRecoveryTimeout defines how long a recovery mark can exist before
	// being considered stale and cleared. This prevents permanent blocking
	// of recovery operations due to crashes or bugs.
	// Defaults to 20 minutes.
	StaleRecoveryTimeout time.Duration `yaml:"staleRecoveryTimeout" default:"20m"`

	// StaleRecoveryCleanupSchedule defines the cron schedule for stale recovery cleanup.
	// Supports cron expressions and intervals (e.g., "@every 1m", "0 * * * * *").
	// Defaults to "@every 1m".
	StaleRecoveryCleanupSchedule string `yaml:"staleRecoveryCleanupSchedule" default:"@every 1m"`
}

// DefaultNatsRecovery returns a NatsRecovery configuration with default values.
func DefaultNatsRecovery() NatsRecovery {
	return NatsRecovery{
		Enabled:                      defaultRecoveryEnabled,
		HealthCheckSchedule:          defaultRecoveryHealthCheckSchedule,
		MaxRecoveryAttempts:          defaultRecoveryMaxAttempts,
		RecoveryBackoff:              defaultRecoveryBackoff,
		StaleRecoveryTimeout:         defaultRecoveryStaleTimeout,
		StaleRecoveryCleanupSchedule: defaultRecoveryStaleCleanupSchedule,
	}
}

// Validate performs validation of the NatsRecovery configuration.
func (c NatsRecovery) Validate() error {
	return ValidateStructIfEnabled(c.Enabled, &c,
		validation.Field(&c.HealthCheckSchedule, validation.Required),
		validation.Field(&c.MaxRecoveryAttempts,
			validation.Min(1).Error("must be at least 1"),
			validation.Max(maxRecoveryAttempts).Error("must be at most 100")),
		validation.Field(&c.RecoveryBackoff, ozzo_rules.Duration()),
		validation.Field(&c.StaleRecoveryTimeout, ozzo_rules.Duration()),
		validation.Field(&c.StaleRecoveryCleanupSchedule, validation.Required),
	)
}
