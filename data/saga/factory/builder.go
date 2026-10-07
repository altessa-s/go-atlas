// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/observability/metrics"

	sagaconfig "github.com/altessa-s/go-atlas/config/saga"
	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	corefactory "github.com/altessa-s/go-atlas/core/factory"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	memorystore "github.com/altessa-s/go-atlas/data/saga/storages/memory"
	mongostore "github.com/altessa-s/go-atlas/data/saga/storages/mongo"
	natsstore "github.com/altessa-s/go-atlas/data/saga/storages/nats"
	redisstore "github.com/altessa-s/go-atlas/data/saga/storages/redis"
	sqlstore "github.com/altessa-s/go-atlas/data/saga/storages/sqldb"
	goredis "github.com/redis/go-redis/v9"
	mongodriver "go.mongodb.org/mongo-driver/v2/mongo"
)

// Builder assembles a [saga.Orchestrator] from a [sagaconfig.Config] and injected
// backend clients, using a fluent API. Create instances with [New]. The state
// store is selected by sagaconfig.Config.Storage.Type; the matching client must be
// injected (UseJetStream / UseMongoDatabase / UseRedisClient / UseSQLDB) or
// Build fails.
//
// The builder is generic over the saga's shared data type T so it can return a
// fully typed orchestrator. It is not safe for concurrent use.
type Builder[T any] struct {
	corefactory.Base
	cfg  *sagaconfig.Config
	def  *saga.Definition[T]
	errs []error

	// Backend clients (only the one named by the storage type is required).
	js          jetstream.JetStream
	mongoDB     *mongodriver.Database
	redisClient goredis.UniversalClient
	sqlDB       *sql.DB

	// Optional orchestrator dependencies.
	collector     metrics.Collector
	scheduler     corescheduler.TaskRegistrar
	leaderElector saga.LeaderElector
	serializer    serializer.Serializer
	onDeadLetter  saga.DeadLetterFunc
	shouldRetry   func(error) bool
}

// New creates a Builder for the given saga config and definition. Both may be
// nil — the error surfaces at [Builder.Build] time.
func New[T any](cfg *sagaconfig.Config, def *saga.Definition[T]) *Builder[T] {
	return &Builder[T]{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
		def:  def,
	}
}

// Build validates the configuration, constructs the configured state store
// (requiring the matching injected client), and returns a typed orchestrator.
func (b *Builder[T]) Build() (*saga.Orchestrator[T], error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}
	if b.cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if b.def == nil {
		return nil, fmt.Errorf("definition is required")
	}

	b.cfg.Normalize()
	if err := b.cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate saga config: %w", err)
	}

	store, err := b.buildStore()
	if err != nil {
		return nil, err
	}

	o := saga.New(store, b.def, b.orchestratorOptions()...)
	if err := o.RegisterRecovery(context.Background()); err != nil {
		return nil, fmt.Errorf("register saga recovery: %w", err)
	}
	return o, nil
}

// buildStore constructs the [saga.Storage] named by the storage type, validating
// that the required backend client was injected.
func (b *Builder[T]) buildStore() (saga.Storage, error) {
	storage := b.cfg.Storage
	switch storage.Type {
	case sagaconfig.StorageTypeMemory:
		return memorystore.New(), nil
	case sagaconfig.StorageTypeNATS:
		if err := b.RequireDependency(b.js, "NATS JetStream"); err != nil {
			return nil, err
		}
		opts := []natsstore.Option{
			natsstore.WithBucket(storage.NATS.Bucket),
			natsstore.WithBucketTTL(storage.NATS.MaxAge),
		}
		opts = coreslices.AppendIf(opts, storage.NATS.MigrateBucketTTL, natsstore.WithMigrateBucketTTL())
		opts = coreslices.AppendIf(opts, storage.NATS.StrictBucketStorage, natsstore.WithStrictBucketStorage())
		return natsstore.New(b.js, opts...)
	case sagaconfig.StorageTypeMongo:
		if err := b.RequireDependency(b.mongoDB, "MongoDB database"); err != nil {
			return nil, err
		}
		return mongostore.New(b.mongoDB, mongostore.WithCollectionName(storage.Mongo.Collection))
	case sagaconfig.StorageTypeRedis:
		if err := b.RequireDependency(b.redisClient, "Redis client"); err != nil {
			return nil, err
		}
		return redisstore.New(b.redisClient,
			redisstore.WithKeyPrefix(storage.Redis.KeysPrefix),
			redisstore.WithTTL(storage.Redis.TTL),
		), nil
	case sagaconfig.StorageTypeSQL:
		if err := b.RequireDependency(b.sqlDB, "SQL database"); err != nil {
			return nil, err
		}
		return b.buildSQLStore(storage.SQL)
	default:
		return nil, fmt.Errorf("unsupported saga storage type %q", storage.Type)
	}
}

// ensureSchemaTimeout bounds the schema creation buildSQLStore runs when the
// config asks for it, so a lock wait cannot stall startup indefinitely.
const ensureSchemaTimeout = 30 * time.Second

// buildSQLStore creates the SQL store. With EnsureSchema set it creates the
// table through the store's idempotent EnsureSchema; otherwise it performs no
// I/O and the schema is expected to exist.
func (b *Builder[T]) buildSQLStore(cfg *sagaconfig.SQLStorageConfig) (*sqlstore.Store, error) {
	store, err := sqlstore.New(b.sqlDB, sqlstore.Dialect(cfg.Dialect), sqlstore.WithTableName(cfg.Table))
	if err != nil || !cfg.EnsureSchema {
		return store, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), ensureSchemaTimeout)
	defer cancel()
	if err := store.EnsureSchema(ctx); err != nil {
		return nil, b.WrapError(err, "ensure saga schema")
	}
	return store, nil
}

// orchestratorOptions translates the config and injected dependencies into
// orchestrator options. The generated With* options are nil/zero-guarded, so
// unset dependencies and zero values are safely ignored.
func (b *Builder[T]) orchestratorOptions() []saga.Option {
	cfg := b.cfg
	return []saga.Option{
		saga.WithLogger(b.Logger()),
		saga.WithCollector(b.collector),
		saga.WithScheduler(b.scheduler),
		saga.WithLeaderElector(b.leaderElector),
		saga.WithSerializer(b.serializer),
		saga.WithOnDeadLetter(b.onDeadLetter),
		saga.WithShouldRetry(b.shouldRetry),
		saga.WithStepTimeout(cfg.StepTimeout),
		saga.WithExecutionTimeout(cfg.ExecutionTimeout),
		saga.WithLeaseGrace(cfg.LeaseGrace),
		saga.WithStorageTimeout(cfg.StorageTimeout),
		saga.WithRecoveryTimeout(cfg.RecoveryTimeout),
		saga.WithSagaTimeout(cfg.SagaTimeout),
		saga.WithMaxStepAttempts(cfg.MaxStepAttempts),
		saga.WithStepRetryBaseDelay(cfg.StepRetryBaseDelay),
		saga.WithStepRetryMaxDelay(cfg.StepRetryMaxDelay),
		saga.WithMaxCompensationAttempts(cfg.MaxCompensationAttempts),
		saga.WithStepConcurrency(cfg.StepConcurrency),
		saga.WithRecoverySchedule(cfg.RecoverySchedule),
		saga.WithRecoveryBatchSize(cfg.RecoveryBatchSize),
		saga.WithRecoveryTaskID(cfg.RecoveryTaskID),
	}
}
