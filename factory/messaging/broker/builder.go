// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"database/sql"
	"log/slog"

	"github.com/altessa-s/go-atlas-core/collections/slices"
	"github.com/altessa-s/go-atlas/messaging/broker"
	"github.com/altessa-s/go-atlas/messaging/broker/inprogress"
	"github.com/altessa-s/go-atlas/messaging/broker/outbox"
	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/observability/tracing"

	coreruntime "github.com/altessa-s/go-atlas-core/runtime"
	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	outboxfactory "github.com/altessa-s/go-atlas/factory/reliability/outbox"
	dataoutbox "github.com/altessa-s/go-atlas/reliability/outbox"
	outboxsql "github.com/altessa-s/go-atlas/reliability/outbox/storages/sqldb"
	schedtask "github.com/altessa-s/go-horai/task"
)

// ProviderEnv carries the builder's shared dependencies to a [ProviderFunc].
type ProviderEnv struct {
	// Logger is the builder's logger.
	Logger *slog.Logger
	// Scheduler is the builder's task scheduler; nil when none is set.
	Scheduler schedtask.Registrar
	// Meter is the builder's metrics meter; nil when none is set.
	Meter metrics.Meter
}

// ProviderFunc creates the broker provider of one provider type from the
// broker configuration.
type ProviderFunc func(cfg *brokerconfig.Config, env ProviderEnv) (broker.Provider, error)

// BrokerBuilder assembles a [broker.Broker] and related components step by step
// using a fluent API. Create instances with [New]. Errors are accumulated and
// reported at [BrokerBuilder.Build] time. The builder is not safe for concurrent use.
//
// The builder builds no broker provider itself. The NATS one registers its
// [ProviderFunc] with [BrokerBuilder.UseProvider] from
// factory/messaging/broker/providers/nats, and [BrokerBuilder.CreateProvider]
// fails for a configured type without one, naming the package to add.
type BrokerBuilder struct {
	corefactory.Base
	cfg  *brokerconfig.Config
	errs []error

	// Dependencies
	publishConverter broker.PublishConverter
	scheduler        schedtask.Registrar
	collector        metrics.Meter
	tracer           tracing.Tracer
	providers        map[brokerconfig.Provider]ProviderFunc
	outboxStorages   map[brokerconfig.OutboxStorageType]outboxfactory.StorageFunc
	sqlDB            *sql.DB
	resolveTx        func(context.Context) (*sql.Tx, bool)
	sqlListener      outboxsql.Listener
	shutdownHooks    *coreruntime.HookGroup
}

// New creates a new [BrokerBuilder] for the given broker config.
// Config can be nil -- the error surfaces at [BrokerBuilder.Build] time.
func New(cfg *brokerconfig.Config) *BrokerBuilder {
	return &BrokerBuilder{
		Base: corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:  cfg,
	}
}

// Build assembles the broker with the given provider. Errors from fluent
// methods are accumulated and reported here via [errors.Join]. The broker
// records its publish metrics on the [BrokerBuilder.UseMeter] meter.
func (b *BrokerBuilder) Build(provider broker.Provider) (*broker.Broker, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	if err := b.RequireDependency(provider, "broker provider"); err != nil {
		return nil, err
	}

	opts := b.applyBrokerDefaults()
	return broker.New(provider, opts...), nil
}

// CreateInProgressManager creates a new InProgress heartbeat manager from the
// builder's broker configuration. When a scheduler is configured, the builder
// automatically registers RunTickCycle task for periodic heartbeat execution.
// The manager records its metrics on the [BrokerBuilder.UseMeter] meter unless
// inProgress.metrics.enabled is false.
func (b *BrokerBuilder) CreateInProgressManager() (*inprogress.Manager, error) {
	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	cfg := &b.cfg.InProgress
	if !cfg.Enabled {
		return nil, nil //nolint:nilnil
	}

	opts := make([]inprogress.Option, 0, 4)
	opts = append(opts, inprogress.WithLogger(b.Logger()), inprogress.WithHeartbeatTimeout(cfg.HeartbeatTimeout))
	opts = slices.AppendIf(opts, b.scheduler != nil,
		inprogress.WithScheduler(b.scheduler),
		inprogress.WithTickSchedule(cfg.TickSchedule),
	)
	opts = slices.AppendIf(opts, cfg.Metrics.Enabled, inprogress.WithMeter(b.collector))

	return inprogress.New(opts...), nil
}

// CreateOutbox creates the outbox over the storage selected by
// outbox.storage.type, using the builder's broker configuration: the sqldb
// store over the handle given to [BrokerBuilder.UseSQLDB], or the one whose
// function was registered with [BrokerBuilder.UseOutboxStorage], for example
// the MongoDB one from factory/reliability/outbox/storages/mongo. With
// outbox.watch set to changeStream or notify the watcher is started as
// described for the reliability/outbox factory. ctx bounds the storage's
// startup I/O and the task registration. Returns (nil, nil) when the outbox is
// disabled.
func (b *BrokerBuilder) CreateOutbox(ctx context.Context, publisher outbox.Publisher) (*outbox.Outbox, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}
	ob, err := b.outboxBuilder(publisher)
	if ob == nil {
		return nil, err
	}
	var system string
	if b.cfg != nil {
		system = string(b.cfg.Provider)
	}
	handler := outbox.NewHandler(publisher, outbox.WithTracer(b.tracer), outbox.WithMessagingSystem(system))
	return wrapOutbox(ob.Build(ctx, handler))
}

// CreateProvider creates the broker provider of the configured provider type
// with the [ProviderFunc] registered by [BrokerBuilder.UseProvider]. Pass the
// result to [BrokerBuilder.Build].
func (b *BrokerBuilder) CreateProvider() (broker.Provider, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}
	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	if build, ok := b.providers[b.cfg.Provider]; ok {
		return build(b.cfg, ProviderEnv{Logger: b.Logger(), Scheduler: b.scheduler, Meter: b.collector})
	}
	if b.cfg.Provider == brokerconfig.ProviderNATS {
		return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "broker provider %q is not registered: add factory/messaging/broker/providers/nats "+
			"with UseProvider(%q, natsfactory.Builder(conn))", b.cfg.Provider, b.cfg.Provider)
	}
	return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unsupported provider type: %s", b.cfg.Provider)
}

// outboxBuilder returns a reliability/outbox builder configured from the broker
// outbox config, with the broker retry classification layered on top. An
// enabled outbox requires a publisher; a nil builder means err holds that
// failure.
func (b *BrokerBuilder) outboxBuilder(publisher outbox.Publisher) (*outboxfactory.OutboxBuilder, error) {
	var cfg *brokerconfig.Outbox
	if b.cfg != nil {
		cfg = &b.cfg.Outbox
	}
	if cfg != nil && cfg.Enabled {
		if err := b.RequireDependency(publisher, "outbox publisher"); err != nil {
			return nil, err
		}
	}

	ob := outboxfactory.New(cfg).
		UseLogger(b.Logger()).
		UseScheduler(b.scheduler).
		UseSQLPool(b.sqlDB, b.resolveTx).
		UseSQLListener(b.sqlListener).
		UseShutdownHooks(b.shutdownHooks).
		UseOutboxOptions(dataoutbox.WithShouldRetry(outbox.ShouldRetry))
	for typ, build := range b.outboxStorages {
		ob.UseStorage(typ, build)
	}
	return ob, nil
}

// wrapOutbox adapts a reliability/outbox factory result to the broker adapter. A nil
// outbox (disabled config, failed build) stays nil; a registration failure
// keeps both the outbox and the error so the caller can retry RegisterTasks.
func wrapOutbox(ob *dataoutbox.Outbox, err error) (*outbox.Outbox, error) {
	if ob == nil {
		return nil, err
	}
	return &outbox.Outbox{Outbox: ob}, err
}

// applyBrokerDefaults returns factory default broker options.
func (b *BrokerBuilder) applyBrokerDefaults() []broker.Option {
	opts := []broker.Option{broker.WithLogger(b.Logger()), broker.WithTracer(b.tracer), broker.WithMessagingSystem(string(b.cfg.Provider))}
	opts = slices.AppendIf(opts, b.publishConverter != nil, broker.WithPublishConverter(b.publishConverter))
	opts = slices.AppendIf(opts, b.collector != nil, broker.WithMeter(b.collector))
	return opts
}
