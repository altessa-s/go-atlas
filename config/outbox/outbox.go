// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxconfig

import (
	"errors"
	"time"

	"github.com/altessa-s/go-atlas/config/internal/cfgerrors"
	"github.com/altessa-s/go-atlas/config/internal/validators"

	validationconfig "github.com/altessa-s/go-nomos/validation"
	ozzo_rules "github.com/altessa-s/ozzo-rules"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Default values for the outbox schema.
const (
	defaultEnabled                 = true
	defaultFetchTimeout            = 5 * time.Second
	defaultHandleTimeout           = 20 * time.Second
	defaultUpdateTimeout           = 5 * time.Second
	defaultMessagesBatchSize       = 200
	defaultRetryMaxAttempts        = 10
	defaultPublishedEventsLifetime = -1
	defaultTopicCompaction         = false
	defaultRetryBaseDelay          = time.Second
	defaultRetryMaxDelay           = 5 * time.Minute
	// Must stay above defaultHandleTimeout — see Config.MaxLockTime.
	defaultMaxLockTime      = 2 * defaultHandleTimeout
	defaultMaxPayloadBytes  = 1 << 20
	defaultDispatchSchedule = "@every 2s"
	defaultCleanupSchedule  = "@every 10m"
	defaultUnlockSchedule   = "@every 11s"
	defaultStatsSchedule    = "@every 30s"
	defaultExpireSchedule   = "@every 11s"
	defaultDispatchTaskID   = "outbox-dispatch"
	defaultUnlockTaskID     = "outbox-unlock"
	defaultExpireTaskID     = "outbox-expire"
	defaultCleanupTaskID    = "outbox-cleanup"
	defaultStatsTaskID      = "outbox-stats"
	defaultMongoCollection  = "events_outbox"
	defaultSQLTable         = "events_outbox"

	// maxMessagesBatchSize is the maximum allowed batch size.
	maxMessagesBatchSize = 10000
	// maxRetryMaxAttempts is the maximum allowed retry attempts.
	maxRetryMaxAttempts = 100
)

// StorageType selects the backend that persists outbox events.
type StorageType string

const (
	// StorageTypeMongo persists events in a MongoDB collection
	// (reliability/outbox/storages/mongo).
	StorageTypeMongo StorageType = "mongo"
	// StorageTypeSQL persists events in a SQL table (PostgreSQL, MySQL
	// or MariaDB) through an injected *sql.DB (reliability/outbox/storages/sqldb).
	StorageTypeSQL StorageType = "sqldb"
)

var outboxStorageAllowedTypes = []StorageType{StorageTypeMongo, StorageTypeSQL}

// Watch selects how the dispatcher learns of newly saved events beyond
// its poll schedule, which stays on in every mode.
type Watch string

const (
	// WatchPolling relies on the dispatch schedule alone.
	WatchPolling Watch = "polling"
	// WatchChangeStream wakes the dispatcher from a MongoDB change
	// stream. MongoDB storage only.
	WatchChangeStream Watch = "changeStream"
	// WatchNotify wakes the dispatcher through PostgreSQL LISTEN/NOTIFY:
	// saves signal the channel, a listener injected into the factory receives
	// it. sqldb storage with the postgres dialect only.
	WatchNotify Watch = "notify"
)

// ErrUnsupportedWatch reports a watch mode the configured storage cannot
// serve: changeStream on sqldb, notify on mongo or on the mysql dialect. It is
// wrapped with go-atlas-core ErrInvalidArgument.
var ErrUnsupportedWatch = errors.New("outbox watch mode is not supported by the storage")

// ErrWatchNeedsSchedule reports a push watch mode without a dispatch
// schedule. Retry backoffs and unlock sweeps still require polling.
var ErrWatchNeedsSchedule = errors.New("outbox watch requires a dispatch schedule")

// MongoStorage configures the MongoDB outbox store
// (reliability/outbox/storages/mongo).
type MongoStorage struct {
	// Collection is the MongoDB collection that stores outbox events.
	Collection string `yaml:"collection" default:"events_outbox"`
}

// Validate performs validation of the MongoDB outbox storage configuration.
func (c *MongoStorage) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Collection, validation.Required),
	)
}

// SQLStorage configures the SQL outbox store
// (reliability/outbox/storages/sqldb). The *sql.DB itself is injected into the
// factory; the caller chooses and registers the driver.
type SQLStorage struct {
	// Dialect selects the SQL flavor: "postgres" or "mysql" (MySQL 8.0+ and
	// MariaDB 10.6+).
	Dialect string `yaml:"dialect" default:"postgres"`
	// Table is the table that stores outbox events, optionally
	// schema-qualified.
	Table string `yaml:"table" default:"events_outbox"`
	// NotifyChannel overrides the PostgreSQL notification channel for watch notify.
	// Empty derives a lower-case channel from the table name. Names are case-sensitive.
	NotifyChannel string `yaml:"notifyChannel"`
	// EnsureSchema makes the factory create the events table and its indexes
	// while building the outbox, through the store's idempotent EnsureSchema.
	// Leave it false when the schema is applied through migrations.
	EnsureSchema bool `yaml:"ensureSchema" default:"false"`
	// KeyOrder claims one event at a time per ordering key, in insertion order.
	KeyOrder bool `yaml:"keyOrder" default:"false"`
	// AsynchronousCommit allows PostgreSQL to flush dispatcher bookkeeping after commit.
	AsynchronousCommit bool `yaml:"asynchronousCommit" default:"false"`
}

// Validate performs validation of the SQL outbox storage configuration.
func (c *SQLStorage) Validate() error {
	return validationconfig.ValidateStruct(c,
		validation.Field(&c.Dialect, validation.Required, ozzo_rules.OneOf(SQLDialectPostgres, SQLDialectMySQL)),
		validation.Field(&c.Table, validation.Required, validators.SQLTableName),
		validation.Field(&c.NotifyChannel, validators.SQLIdentifier),
	)
}

// SQL dialects accepted by [SQLStorage.Dialect].
const (
	// SQLDialectPostgres targets PostgreSQL 12+.
	SQLDialectPostgres = "postgres"
	// SQLDialectMySQL targets MySQL 8.0+ and MariaDB 10.6+.
	SQLDialectMySQL = "mysql"
)

// Storage selects and configures the outbox storage backend. Each
// backend has its own nested section; only the one named by Type is used.
type Storage struct {
	validators.BackendTypes[StorageType] `yaml:"-" json:"-"`

	// Type selects mongo, sqldb, or a registered storage.
	Type StorageType `yaml:"type" default:"mongo"`

	// Mongo defines the MongoDB configuration.
	// Required when Type is StorageTypeMongo, ignored otherwise.
	Mongo *MongoStorage `yaml:"mongo" default:"-"`

	// SQL defines the SQL database configuration.
	// Required when Type is StorageTypeSQL, ignored otherwise.
	SQL *SQLStorage `yaml:"sqldb" default:"-"`
}

// DefaultStorage returns the default outbox storage configuration
// (MongoDB, default collection).
func DefaultStorage() *Storage {
	return &Storage{
		Type:  StorageTypeMongo,
		Mongo: &MongoStorage{Collection: defaultMongoCollection},
	}
}

// Normalize allocates the backend sub-config implied by
// [Storage.Type] with its defaults. Call Normalize before Validate
// so the correct sub-struct is present for validation.
func (c *Storage) Normalize() {
	switch c.Type {
	case StorageTypeMongo:
		if c.Mongo == nil {
			c.Mongo = &MongoStorage{Collection: defaultMongoCollection}
		}
	case StorageTypeSQL:
		if c.SQL == nil {
			c.SQL = &SQLStorage{Dialect: SQLDialectPostgres, Table: defaultSQLTable}
		}
	}
}

func (c *Storage) storageCases() []validationconfig.StorageCase[StorageType] {
	return []validationconfig.StorageCase[StorageType]{
		{When: StorageTypeMongo, Field: &c.Mongo},
		{When: StorageTypeSQL, Field: &c.SQL},
	}
}

// Validate performs validation of the outbox storage configuration. Ensures
// the storage type is valid and its corresponding configuration is provided.
func (c *Storage) Validate() error {
	return validationconfig.ValidateStorage(c, &c.Type, c.AllowedTypes(outboxStorageAllowedTypes...), c.storageCases())
}

// Config defines the configuration for reliable event delivery via the outbox pattern.
// This configuration is shared by both the generic outbox (reliability/outbox) and
// the broker-specific adapter (messaging/broker/outbox).
type Config struct {
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
	Storage *Storage `yaml:"storage"`

	// Watch selects how the dispatcher learns of newly saved events beyond its
	// poll schedule: polling (the schedule alone), changeStream (MongoDB
	// storage) or notify (PostgreSQL LISTEN/NOTIFY, sqldb storage with the
	// postgres dialect). The factory starts the watcher; the dispatch schedule
	// stays mandatory, since retry backoffs and reclaimed leases produce no
	// notification. Defaults to polling.
	Watch Watch `yaml:"watch" default:"polling"`

	// Scheduler task IDs. Set these to distinct non-default values when
	// running multiple outbox instances against the same scheduler — the
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

// Default returns the outbox configuration with its defaults applied.
func Default() Config {
	return Config{
		Enabled:                 defaultEnabled,
		FetchTimeout:            defaultFetchTimeout,
		HandleTimeout:           defaultHandleTimeout,
		UpdateTimeout:           defaultUpdateTimeout,
		MessagesBatchSize:       defaultMessagesBatchSize,
		RetryMaxAttempts:        defaultRetryMaxAttempts,
		PublishedEventsLifetime: defaultPublishedEventsLifetime,
		TopicCompaction:         defaultTopicCompaction,
		DispatchSchedule:        defaultDispatchSchedule,
		CleanupSchedule:         defaultCleanupSchedule,
		UnlockSchedule:          defaultUnlockSchedule,
		StatsSchedule:           defaultStatsSchedule,
		ExpireSchedule:          defaultExpireSchedule,
		RetryBaseDelay:          defaultRetryBaseDelay,
		RetryMaxDelay:           defaultRetryMaxDelay,
		MaxLockTime:             defaultMaxLockTime,
		MaxPayloadBytes:         defaultMaxPayloadBytes,
		DispatchTaskID:          defaultDispatchTaskID,
		UnlockTaskID:            defaultUnlockTaskID,
		ExpireTaskID:            defaultExpireTaskID,
		CleanupTaskID:           defaultCleanupTaskID,
		StatsTaskID:             defaultStatsTaskID,
		Storage:                 DefaultStorage(),
		Watch:                   WatchPolling,
	}
}

// Normalize defaults a missing storage section to [DefaultStorage] and
// allocates the backend sub-config implied by its type. Call before
// [Config.Validate].
func (c *Config) Normalize() {
	if c.Storage == nil {
		c.Storage = DefaultStorage()
	}
	c.Storage.Normalize()
}

// ValidateStorage validates the storage section and the watch mode alone — the
// part of [Config.Validate] the outbox factories check before building the
// store. A push mode without a dispatch schedule fails
// with [ErrWatchNeedsSchedule].
func (c *Config) ValidateStorage() error {
	err := validationconfig.ValidateStruct(c, validation.Field(&c.Storage, validation.Required))
	if c.Watch != "" {
		watchErr := validationconfig.ValidateStruct(c,
			validation.Field(&c.Watch, ozzo_rules.OneOf(WatchPolling, WatchChangeStream, WatchNotify)))
		if watchErr != nil {
			err = errors.Join(err, watchErr, cfgerrors.Invalidf("outbox watch %q: %w", c.Watch, ErrUnsupportedWatch))
		}
	}
	if (c.Watch == WatchChangeStream || c.Watch == WatchNotify) && c.DispatchSchedule == "" {
		err = errors.Join(err, cfgerrors.Invalidf("outbox watch %q: %w", c.Watch, ErrWatchNeedsSchedule))
	}
	return err
}

// Validate checks the outbox schema, including
// [Config.ValidateStorage]. A disabled outbox is not validated.
func (c Config) Validate() error {
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
		validation.Field(&c.MessagesBatchSize, validation.Required, validation.Min(uint32(1)), validation.Max(uint32(maxMessagesBatchSize))),
		validation.Field(&c.RetryMaxAttempts, validation.Required, validation.Min(uint32(1)), validation.Max(uint32(maxRetryMaxAttempts))),
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
