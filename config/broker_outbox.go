// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config

import (
	"time"

	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for Outbox configuration.
const (
	defaultOutboxEnabled                 = true
	defaultOutboxFetchTimeout            = 5 * time.Second
	defaultOutboxHandleTimeout           = 20 * time.Second
	defaultOutboxUpdateTimeout           = 5 * time.Second
	defaultOutboxMessagesBatchSize       = 200
	defaultOutboxRetryMaxAttempts        = 10
	defaultOutboxPublishedEventsLifetime = -1
	defaultOutboxTopicCompaction         = false

	// maxOutboxMessagesBatchSize is the maximum allowed batch size.
	maxOutboxMessagesBatchSize = 10000
	// maxOutboxRetryMaxAttempts is the maximum allowed retry attempts.
	maxOutboxRetryMaxAttempts = 100
)

// Outbox defines the configuration for reliable event delivery via the outbox pattern.
// This configuration is shared by both the generic outbox (data/outbox) and
// the broker-specific adapter (transport/broker/outbox).
type Outbox struct {
	// Enabled determines whether the outbox is enabled.
	// Defaults to true.
	Enabled bool `yaml:"enabled" default:"true"`

	// DispatchSchedule defines the cron schedule for the dispatcher.
	// Example: "*/2 * * * * *" for every 2 seconds.
	DispatchSchedule string `yaml:"dispatchSchedule" default:"@every 2s"`

	// FetchTimeout defines the timeout for fetching unprocessed events.
	// Defaults to 5 seconds.
	FetchTimeout time.Duration `yaml:"fetchTimeout" default:"5s"`

	// HandleTimeout defines the timeout for processing a batch of events.
	// Defaults to 20 seconds.
	HandleTimeout time.Duration `yaml:"handleTimeout" default:"20s"`

	// UpdateTimeout defines the timeout for updating event status.
	// Defaults to 5 seconds.
	UpdateTimeout time.Duration `yaml:"updateTimeout" default:"5s"`

	// CleanupSchedule defines the cron schedule for the cleanup task.
	// Example: "0 */10 * * * *" for every 10 minutes.
	CleanupSchedule string `yaml:"cleanupSchedule" default:"@every 10m"`

	// UnlockSchedule defines the cron schedule for the unlocker task.
	// Example: "*/11 * * * * *" for every 11 seconds.
	UnlockSchedule string `yaml:"unlockSchedule" default:"@every 11s"`

	// MessagesBatchSize defines the number of events fetched per batch.
	// Must be between 1 and 10000 to prevent memory exhaustion.
	// Defaults to 200.
	MessagesBatchSize uint32 `yaml:"messagesBatchSize" default:"200"`

	// RetryMaxAttempts defines the maximum number of publish attempts.
	// Must be between 1 and 100 to prevent infinite retry loops.
	// Defaults to 10.
	RetryMaxAttempts uint32 `yaml:"retryMaxAttempts" default:"10"`

	// PublishedEventsLifetime defines how long published events are retained.
	// -1 means indefinite. Defaults to -1.
	PublishedEventsLifetime time.Duration `yaml:"publishedEventsLifetime" default:"-1"`

	// TopicCompaction enables log compaction behavior.
	// Defaults to false.
	TopicCompaction bool `yaml:"topicCompaction" default:"false"`

	// DefaultEventTTL is the default time-to-live for events without an explicit ExpiresAt.
	// 0 means disabled (events never expire). Minimum 1s.
	DefaultEventTTL time.Duration `yaml:"defaultEventTTL" default:"0"`

	// ExpireSchedule defines the cron schedule for the expire task that marks
	// pending/failed events past their ExpiresAt as expired.
	ExpireSchedule string `yaml:"expireSchedule" default:"@every 11s"`

	// Scheduler task IDs. Set these to distinct non-default values when
	// running multiple Outbox instances against the same scheduler — the
	// underlying registrar upserts by ID, so two instances sharing
	// "outbox-dispatch" would silently overwrite each other's Func
	// pointers. The runtime rejects collisions across the four IDs at
	// startup via outbox.ErrTaskIDCollision.
	DispatchTaskID string `yaml:"dispatchTaskID" default:"outbox-dispatch"`
	UnlockTaskID   string `yaml:"unlockTaskID" default:"outbox-unlock"`
	ExpireTaskID   string `yaml:"expireTaskID" default:"outbox-expire"`
	CleanupTaskID  string `yaml:"cleanupTaskID" default:"outbox-cleanup"`
}

// Validate performs validation of the Outbox configuration.
func (c Outbox) Validate() error {
	return ValidateStructIfEnabled(c.Enabled, &c,
		validation.Field(&c.FetchTimeout, validation.Min(time.Millisecond)),
		validation.Field(&c.HandleTimeout, validation.Min(time.Millisecond)),
		validation.Field(&c.UpdateTimeout, validation.Min(time.Millisecond)),
		validation.Field(&c.MessagesBatchSize, validation.Min(uint32(1)), validation.Max(uint32(maxOutboxMessagesBatchSize))),
		validation.Field(&c.RetryMaxAttempts, validation.Min(uint32(1)), validation.Max(uint32(maxOutboxRetryMaxAttempts))),
		validation.Field(&c.DispatchTaskID, validation.Required),
		validation.Field(&c.UnlockTaskID, validation.Required),
		validation.Field(&c.ExpireTaskID, validation.Required),
		validation.Field(&c.CleanupTaskID, validation.Required),
	)
}
