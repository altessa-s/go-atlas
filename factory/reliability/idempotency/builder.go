// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/reliability/idempotency"
	"github.com/altessa-s/go-atlas/reliability/idempotency/principalowner"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"

	coreslices "github.com/altessa-s/go-atlas-core/collections/slices"
	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
	memorystorage "github.com/altessa-s/go-atlas/reliability/idempotency/storages/memory"
	schedtask "github.com/altessa-s/go-horai/task"
)

// StorageFunc creates the idempotency storage of one storage type from the
// idempotency configuration.
type StorageFunc func(cfg *idempotencyconfig.Config) (storages.Storage, error)

// KeeperBuilder assembles an idempotency [idempotency.Keeper] step by step using a fluent API.
// Create instances with [New]. Errors are accumulated and reported at [KeeperBuilder.Build] time.
// The builder is not safe for concurrent use.
//
// The builder builds only the in-memory storage. The Redis, NATS and MongoDB
// ones register their [StorageFunc] with [KeeperBuilder.UseStorage] from
// reliability/idempotency/storages/<backend>/factory; a configured type without one
// fails at Build naming the package to add.
type KeeperBuilder struct {
	corefactory.Base
	cfg  *idempotencyconfig.Config
	errs []error

	storages map[idempotencyconfig.StorageType]StorageFunc

	// Dependencies
	scheduler schedtask.Registrar
	collector metrics.Meter

	// Caller binding of the Keeper; owner defaults to principalowner.PrincipalOwner.
	owner            idempotency.OwnerFunc
	keyNamespace     idempotency.KeyNamespaceFunc
	replayAuthorizer idempotency.ReplayAuthorizer
}

// New creates a [KeeperBuilder] for the given idempotency config.
// Config can be nil — the error surfaces at [KeeperBuilder.Build] time.
// The Keeper binds each record to the verified principal by default
// ([principalowner.PrincipalOwner]); see [KeeperBuilder.UseOwner].
func New(cfg *idempotencyconfig.Config) *KeeperBuilder {
	return &KeeperBuilder{
		Base:  corefactory.NewBase(slog.New(slog.DiscardHandler)),
		cfg:   cfg,
		owner: principalowner.PrincipalOwner,
	}
}

// Build assembles the idempotency Keeper. Errors from fluent methods are accumulated
// and reported here via [errors.Join].
func (b *KeeperBuilder) Build() (*idempotency.Keeper, error) {
	if err := corefactory.JoinErrors(b.errs); err != nil {
		return nil, err
	}

	if b.cfg == nil {
		return nil, corefactory.ErrConfigRequired
	}

	storage, err := b.createStorage()
	if err != nil {
		return nil, err
	}

	return b.createKeeper(storage), nil
}

// createKeeper creates a Keeper with the given storage.
func (b *KeeperBuilder) createKeeper(storage storages.Storage) *idempotency.Keeper {
	opts := []idempotency.Option{
		idempotency.WithLogger(b.Logger()),
		idempotency.WithMeter(b.collector),
		idempotency.WithOwner(b.owner),
		idempotency.WithKeyNamespace(b.keyNamespace),
		idempotency.WithReplayAuthorizer(b.replayAuthorizer),
	}
	opts = coreslices.AppendIf(opts, b.cfg.MaxLockDuration > 0,
		idempotency.WithMaxLockDuration(b.cfg.MaxLockDuration))
	return idempotency.New(storage, opts...)
}

// createStorage creates the in-memory storage or the one registered for the
// configured type.
func (b *KeeperBuilder) createStorage() (storages.Storage, error) {
	if b.cfg.Storage == nil {
		return nil, corefactory.ErrConfigRequired
	}

	switch b.cfg.Storage.Type {
	case idempotencyconfig.StorageTypeMemory:
		return b.createMemoryStorage()
	case idempotencyconfig.StorageTypeRedis:
		return b.createRegisteredStorage("client")
	case idempotencyconfig.StorageTypeNATS:
		return b.createRegisteredStorage("js")
	case idempotencyconfig.StorageTypeMongo:
		return b.createRegisteredStorage("db")
	default:
		return nil, corefactory.Errorf(corefactory.ErrUnsupported, "unsupported storage type: %s", b.cfg.Storage.Type)
	}
}

// createRegisteredStorage runs the [StorageFunc] registered for the configured
// type; without one it names the package to add and the client its Builder takes.
func (b *KeeperBuilder) createRegisteredStorage(client string) (storages.Storage, error) {
	typ := b.cfg.Storage.Type
	if build, ok := b.storages[typ]; ok {
		return build(b.cfg)
	}
	return nil, corefactory.Errorf(corefactory.ErrNotRegistered, "idempotency storage %q is not registered: add factory/reliability/idempotency/storages/%s "+
		"with UseStorage(%q, %sfactory.Builder(%s))", typ, typ, typ, typ, client)
}

// createMemoryStorage creates an in-memory storage from configuration.
func (b *KeeperBuilder) createMemoryStorage() (*memorystorage.Storage, error) {
	if b.cfg.Storage.Memory == nil {
		return nil, corefactory.ErrConfigRequired
	}

	return memorystorage.New(memorystorage.WithTTL(b.cfg.TTL),
		memorystorage.WithCleanupSchedule(b.cfg.Storage.Memory.CleanupSchedule),
		memorystorage.WithScheduler(b.scheduler),
	), nil
}
