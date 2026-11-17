// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package brokerconfig

import (
	"errors"
	"time"

	"github.com/altessa-s/go-atlas/config/internal/cfgerrors"

	validationconfig "github.com/altessa-s/go-nomos/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
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
	defaultOutboxRetryBaseDelay          = time.Second
	defaultOutboxRetryMaxDelay           = 5 * time.Minute
	// Must stay above defaultOutboxHandleTimeout — see Outbox.MaxLockTime.
	defaultOutboxMaxLockTime      = 2 * defaultOutboxHandleTimeout
	defaultOutboxMaxPayloadBytes  = 1 << 20
	defaultOutboxDispatchSchedule = "@every 2s"
	defaultOutboxCleanupSchedule  = "@every 10m"
	defaultOutboxUnlockSchedule   = "@every 11s"
	defaultOutboxStatsSchedule    = "@every 30s"
	defaultOutboxExpireSchedule   = "@every 11s"
	defaultOutboxDispatchTaskID   = "outbox-dispatch"
	defaultOutboxUnlockTaskID     = "outbox-unlock"
	defaultOutboxExpireTaskID     = "outbox-expire"
	defaultOutboxCleanupTaskID    = "outbox-cleanup"
	defaultOutboxStatsTaskID      = "outbox-stats"
	defaultOutboxMongoCollection  = "events_outbox"
	defaultOutboxSQLTable         = "events_outbox"

	// maxOutboxMessagesBatchSize is the maximum allowed batch size.
	maxOutboxMessagesBatchSize = 10000
	// maxOutboxRetryMaxAttempts is the maximum allowed retry attempts.
	maxOutboxRetryMaxAttempts = 100
)

// OutboxStorageType selects the backend that persists outbox events.
type OutboxStorageType string

const (
	// OutboxStorageTypeMongo persists events in a MongoDB collection
	// (reliability/outbox/storages/mongo).
	OutboxStorageTypeMongo OutboxStorageType = "mongo"
	// OutboxStorageTypeSQL persists events in a SQL table (PostgreSQL, MySQL
	// or MariaDB) through an injected *sql.DB (reliability/outbox/storages/sqldb).
	OutboxStorageTypeSQL OutboxStorageType = "sqldb"
)

var outboxStorageAllowedTypes = []OutboxStorageType{OutboxStorageTypeMongo, OutboxStorageTypeSQL}

// OutboxWatch selects how the dispatcher learns of newly saved events beyond
// its poll schedule, which stays on in every mode.
type OutboxWatch string

const (
	// OutboxWatchPolling relies on the dispatch schedule alone.
	OutboxWatchPolling OutboxWatch = "polling"
	// OutboxWatchChangeStream wakes the dispatcher from a MongoDB change
	// stream. MongoDB storage only.
	OutboxWatchChangeStream OutboxWatch = "changeStream"
	// OutboxWatchNotify wakes the dispatcher through PostgreSQL LISTEN/NOTIFY:
	// saves signal the channel, a listener injected into the factory receives
	// it. sqldb storage with the postgres dialect only.
	OutboxWatchNotify OutboxWatch = "notify"
)

// ErrUnsupportedOutboxWatch reports a watch mode the configured storage cannot
// serve: changeStream on sqldb, notify on mongo or on the mysql dialect. It is
// wrapped with go-atlas-core ErrInvalidArgument.
var ErrUnsupportedOutboxWatch = errors.New("outbox watch mode is not supported by the storage")

// OutboxMongoStorageConfig configures the MongoDB outbox store
// (reliability/outbox/storages/mongo).
type OutboxMongoStorageConfig struct {
	// Collection is the MongoDB collection that stores outbox events.
	Collection string `yaml:"collection" default:"events_outbox"`
}

// Validate performs validation of the MongoDB outbox storage configuration.
func (c *OutboxMongoStorageConfig) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Collection, validation.Required),
	)
}

// OutboxSQLStorageConfig configures the SQL outbox store
// (reliability/outbox/storages/sqldb). The *sql.DB itself is injected into the
// factory; the caller chooses and registers the driver.
type OutboxSQLStorageConfig struct {
	// Dialect selects the SQL flavor: "postgres" or "mysql" (MySQL 8.0+ and
	// MariaDB 10.6+).
	Dialect string `yaml:"dialect" default:"postgres"`
	// Table is the table that stores outbox events, optionally
	// schema-qualified.
	Table string `yaml:"table" default:"events_outbox"`
	// EnsureSchema makes the factory create the events table and its indexes
	// while building the outbox, through the store's idempotent EnsureSchema.
	// Leave it false when the schema is applied through migrations.
	EnsureSchema bool `yaml:"ensureSchema" default:"false"`
}

// Validate performs validation of the SQL outbox storage configuration.
func (c *OutboxSQLStorageConfig) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Dialect, validation.Required, ozzo_rules.OneOf(SQLDialectPostgres, SQLDialectMySQL)),
		validation.Field(&c.Table, validation.Required),
	)
}

// SQL dialects accepted by [OutboxSQLStorageConfig.Dialect].
const (
	// SQLDialectPostgres targets PostgreSQL 12+.
	SQLDialectPostgres = "postgres"
	// SQLDialectMySQL targets MySQL 8.0+ and MariaDB 10.6+.
	SQLDialectMySQL = "mysql"
)

// OutboxStorageConfig selects and configures the outbox storage backend. Each
// backend has its own nested section; only the one named by Type is used.
type OutboxStorageConfig struct {
	// Type defines the storage backend type (mongo or sqldb).
	Type OutboxStorageType `yaml:"type" default:"mongo"`

	// Mongo defines the MongoDB configuration.
	// Required when Type is OutboxStorageTypeMongo, ignored otherwise.
	Mongo *OutboxMongoStorageConfig `yaml:"mongo" default:"-"`

	// SQL defines the SQL database configuration.
	// Required when Type is OutboxStorageTypeSQL, ignored otherwise.
	SQL *OutboxSQLStorageConfig `yaml:"sqldb" default:"-"`
}

// DefaultOutboxStorage returns the default outbox storage configuration
// (MongoDB, default collection).
func DefaultOutboxStorage() *OutboxStorageConfig {
	return &OutboxStorageConfig{
		Type:  OutboxStorageTypeMongo,
		Mongo: &OutboxMongoStorageConfig{Collection: defaultOutboxMongoCollection},
	}
}

// Normalize allocates the backend sub-config implied by
// [OutboxStorageConfig.Type] with its defaults. Call Normalize before Validate
// so the correct sub-struct is present for validation.
func (c *OutboxStorageConfig) Normalize() {
	switch c.Type {
	case OutboxStorageTypeMongo:
		if c.Mongo == nil {
			c.Mongo = &OutboxMongoStorageConfig{Collection: defaultOutboxMongoCollection}
		}
	case OutboxStorageTypeSQL:
		if c.SQL == nil {
			c.SQL = &OutboxSQLStorageConfig{Dialect: SQLDialectPostgres, Table: defaultOutboxSQLTable}
		}
	}
}

func (c *OutboxStorageConfig) storageCases() []validationconfig.StorageCase[OutboxStorageType] {
	return []validationconfig.StorageCase[OutboxStorageType]{
		{When: OutboxStorageTypeMongo, Field: &c.Mongo},
		{When: OutboxStorageTypeSQL, Field: &c.SQL},
	}
}

// Validate performs validation of the outbox storage configuration. Ensures
// the storage type is valid and its corresponding configuration is provided.
func (c *OutboxStorageConfig) Validate() error {
	return validationconfig.ValidateStorage(c, &c.Type, outboxStorageAllowedTypes, c.storageCases())
}

// supportsWatch reports whether the storage can serve the watch mode; the
// empty mode is polling.
func (c *OutboxStorageConfig) supportsWatch(w OutboxWatch) bool {
	switch w {
	case "", OutboxWatchPolling:
		return true
	case OutboxWatchChangeStream:
		return c.Type == OutboxStorageTypeMongo
	case OutboxWatchNotify:
		return c.Type == OutboxStorageTypeSQL && c.SQL != nil && c.SQL.Dialect == SQLDialectPostgres
	default:
		return false
	}
}

// Outbox defines the configuration for reliable event delivery via the outbox pattern.
// This configuration is shared by both the generic outbox (reliability/outbox) and
// the broker-specific adapter (messaging/broker/outbox).
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

	// RetryBaseDelay is the backoff before the second publish attempt.
	// Subsequent attempts back off exponentially with jitter. Defaults to 1s.
	RetryBaseDelay time.Duration `yaml:"retryBaseDelay" default:"1s"`

	// RetryMaxDelay caps the backoff between publish attempts. Defaults to 5m.
	RetryMaxDelay time.Duration `yaml:"retryMaxDelay" default:"5m"`

	// MaxLockTime is how long an event may stay locked by a dispatcher before
	// the unlock task reclaims it. It MUST exceed HandleTimeout — a lock that
	// expires mid-publish lets a second worker pick the same event up, making
	// duplicate delivery routine. Values at or below HandleTimeout are raised
	// at runtime with a warning. Defaults to 40s (2x the default HandleTimeout).
	MaxLockTime time.Duration `yaml:"maxLockTime" default:"40s"`

	// MaxPayloadBytes caps the accepted event payload size; 0 disables the
	// check. Larger messages belong behind a claim-check. Defaults to 1 MiB.
	MaxPayloadBytes int `yaml:"maxPayloadBytes" default:"1048576"`

	// StatsSchedule defines the cron schedule for the stats task that refreshes
	// the backlog, dead-letter, and lag gauges. Alerting on those gauges is what
	// keeps a stalled outbox from looking like an idle one.
	StatsSchedule string `yaml:"statsSchedule" default:"@every 30s"`

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

	// Storage selects and configures the events storage backend.
	Storage *OutboxStorageConfig `yaml:"storage"`

	// Watch selects how the dispatcher learns of newly saved events beyond its
	// poll schedule: polling (the schedule alone), changeStream (MongoDB
	// storage) or notify (PostgreSQL LISTEN/NOTIFY, sqldb storage with the
	// postgres dialect). The factory starts the watcher; the dispatch schedule
	// stays mandatory, since retry backoffs and reclaimed leases produce no
	// notification. Defaults to polling.
	Watch OutboxWatch `yaml:"watch" default:"polling"`

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
	StatsTaskID    string `yaml:"statsTaskID" default:"outbox-stats"`
}

// Normalize defaults a missing storage section to [DefaultOutboxStorage] and
// allocates the backend sub-config implied by its type. Call before
// [Outbox.Validate].
func (c *Outbox) Normalize() {
	if c.Storage == nil {
		c.Storage = DefaultOutboxStorage()
	}
	c.Storage.Normalize()
}

// ValidateStorage validates the storage section and the watch mode alone — the
// part of [Outbox.Validate] the outbox factories check before building the
// store. A watch mode the storage cannot serve fails with
// [ErrUnsupportedOutboxWatch].
func (c *Outbox) ValidateStorage() error {
	err := validationconfig.ValidateStruct(c,
		validation.Field(&c.Storage, validation.Required),
		validation.Field(&c.Watch, validation.When(c.Watch != "", ozzo_rules.OneOf(OutboxWatchPolling, OutboxWatchChangeStream, OutboxWatchNotify))),
	)
	if c.Storage != nil && !c.Storage.supportsWatch(c.Watch) {
		err = errors.Join(err, cfgerrors.Invalidf("outbox watch %q with storage %q: %w", c.Watch, c.Storage.Type, ErrUnsupportedOutboxWatch))
	}
	return err
}

// Validate performs validation of the Outbox configuration, including
// [Outbox.ValidateStorage]. A disabled outbox is not validated.
func (c Outbox) Validate() error {
	if !c.Enabled {
		return nil
	}
	err := validationconfig.ValidateStructIfEnabled(c.Enabled, &c,
		// Required (for the durations: PositiveDuration) is paired with Min
		// because ozzo-validation skips every rule but Required for a zero
		// value — Min alone accepts 0.
		validation.Field(&c.FetchTimeout, validationconfig.PositiveDuration(), validation.Min(time.Millisecond)),
		validation.Field(&c.HandleTimeout, validationconfig.PositiveDuration(), validation.Min(time.Millisecond)),
		validation.Field(&c.UpdateTimeout, validationconfig.PositiveDuration(), validation.Min(time.Millisecond)),
		validation.Field(&c.MessagesBatchSize, validation.Required, validation.Min(uint32(1)), validation.Max(uint32(maxOutboxMessagesBatchSize))),
		validation.Field(&c.RetryMaxAttempts, validation.Required, validation.Min(uint32(1)), validation.Max(uint32(maxOutboxRetryMaxAttempts))),
		validation.Field(&c.RetryBaseDelay, validationconfig.PositiveDuration(), validation.Min(time.Millisecond)),
		validation.Field(&c.RetryMaxDelay, validationconfig.PositiveDuration(), validation.Min(time.Millisecond)),
		validation.Field(&c.MaxLockTime, validationconfig.PositiveDuration(), validation.Min(time.Millisecond)),
		validation.Field(&c.MaxPayloadBytes, validation.Min(0)),
		validation.Field(&c.DispatchTaskID, validation.Required),
		validation.Field(&c.UnlockTaskID, validation.Required),
		validation.Field(&c.ExpireTaskID, validation.Required),
		validation.Field(&c.CleanupTaskID, validation.Required),
		validation.Field(&c.StatsTaskID, validation.Required),
	)
	return errors.Join(err, c.ValidateStorage())
}
