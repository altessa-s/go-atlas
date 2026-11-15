// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package brokerconfig

import (
	"time"

	validationconfig "github.com/altessa-s/go-nomos/validation"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for InProgress heartbeat manager configuration.
const (
	defaultInProgressEnabled        = true
	defaultInProgressTickSchedule   = "*/1 * * * * *" // Every second
	defaultInProgressMetricsEnabled = true

	// defaultInProgressHeartbeatTimeout mirrors
	// messaging/broker/inprogress.DefaultHeartbeatTimeout.
	defaultInProgressHeartbeatTimeout = 5 * time.Second
)

// InProgressMetrics defines the metrics configuration for InProgress heartbeat manager.
type InProgressMetrics struct {
	// Enabled allows enabling or disabling metrics collection: when false,
	// the manager is built without the factory's meter.
	// Defaults to true.
	Enabled bool `yaml:"enabled" default:"true"`
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

	// HeartbeatTimeout bounds a single InProgress call, so that one slow
	// heartbeater cannot hold a tick past the AckWait of the others.
	// 0 keeps the manager default.
	// Defaults to 5 seconds.
	HeartbeatTimeout time.Duration `yaml:"heartbeatTimeout" default:"5s"`

	// Metrics contains metrics-related settings.
	Metrics InProgressMetrics `yaml:"metrics"`
}

// Validate performs validation of the InProgress configuration.
func (c InProgress) Validate() error {
	return validationconfig.ValidateStructIfEnabled(c.Enabled, &c,
		validation.Field(&c.TickSchedule, validation.Required),
		validation.Field(&c.HeartbeatTimeout, validationconfig.NonNegativeDuration()),
	)
}
