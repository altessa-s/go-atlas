// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/metrics"
	"github.com/altessa-s/go-atlas/reliability/idempotency"

	idempotencyconfig "github.com/altessa-s/go-atlas/config/idempotency"
	schedtask "github.com/altessa-s/go-horai/task"
)

// UseLogger sets the logger for the builder and all created components.
func (b *KeeperBuilder) UseLogger(v *slog.Logger) *KeeperBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *KeeperBuilder) UseDefaultLogger() *KeeperBuilder {
	return b.UseLogger(slog.Default())
}

// UseStorage registers the storage function of one idempotency storage type
// (redis, nats or mongo). A later registration of the same type replaces the
// earlier one; the memory storage is built in.
func (b *KeeperBuilder) UseStorage(typ idempotencyconfig.StorageType, build StorageFunc) *KeeperBuilder {
	if build == nil {
		b.errs = append(b.errs, fmt.Errorf("UseStorage %q: nil function", typ))
		return b
	}
	if b.storages == nil {
		b.storages = map[idempotencyconfig.StorageType]StorageFunc{}
	}
	b.storages[typ] = build
	return b
}

// UseScheduler sets the scheduler for background task registration.
func (b *KeeperBuilder) UseScheduler(v schedtask.Registrar) *KeeperBuilder {
	b.scheduler = v
	return b
}

// UseMeter sets the [metrics.Meter] for recording idempotency metrics.
func (b *KeeperBuilder) UseMeter(v metrics.Meter) *KeeperBuilder {
	b.collector = v
	return b
}

// UseOwner sets the [idempotency.OwnerFunc] labeling each lock with its
// caller; the Keeper refuses another caller the record. The default is
// [principalowner.PrincipalOwner], the verified principal of the request.
// Without an owner the transports never replay a record or echo its entity ID.
func (b *KeeperBuilder) UseOwner(v idempotency.OwnerFunc) *KeeperBuilder {
	b.owner = v
	return b
}

// UseKeyNamespace sets the [idempotency.KeyNamespaceFunc] isolating keys per
// context (typically per tenant). The default is the empty namespace.
func (b *KeeperBuilder) UseKeyNamespace(v idempotency.KeyNamespaceFunc) *KeeperBuilder {
	b.keyNamespace = v
	return b
}

// UseReplayAuthorizer sets the [idempotency.ReplayAuthorizer] deciding every
// replay from the recorded owner label instead of comparing labels.
func (b *KeeperBuilder) UseReplayAuthorizer(v idempotency.ReplayAuthorizer) *KeeperBuilder {
	b.replayAuthorizer = v
	return b
}
