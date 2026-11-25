// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/altessa-s/go-atlas/coord/limiters/tokenbucket"
	"github.com/altessa-s/go-atlas/observability/metrics"

	storageconfig "github.com/altessa-s/go-atlas/config/storage"
	storagesfactory "github.com/altessa-s/go-atlas/factory/coord/limiters/storages"
	schedtask "github.com/altessa-s/go-horai/task"
)

// UseLogger sets the logger for the builder and all created components.
func (b *TokenBucketLimiterBuilder) UseLogger(v *slog.Logger) *TokenBucketLimiterBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *TokenBucketLimiterBuilder) UseDefaultLogger() *TokenBucketLimiterBuilder {
	return b.UseLogger(slog.Default())
}

// UseStorage registers the storage function of one storage type
// (storageconfig.BackendTypeRedis, storageconfig.BackendTypeNATS). A later
// registration of the same type replaces the earlier one; the memory storage
// is built in.
func (b *TokenBucketLimiterBuilder) UseStorage(typ storageconfig.BackendType, build storagesfactory.StorageFunc) *TokenBucketLimiterBuilder {
	if build == nil {
		b.errs = append(b.errs, fmt.Errorf("UseStorage %q: nil function", typ))
		return b
	}
	if b.storages == nil {
		b.storages = map[storageconfig.BackendType]storagesfactory.StorageFunc{}
	}
	b.storages[typ] = build
	return b
}

// UseScheduler sets the scheduler for background task registration.
func (b *TokenBucketLimiterBuilder) UseScheduler(v schedtask.Registrar) *TokenBucketLimiterBuilder {
	b.scheduler = v
	return b
}

// UseMeter sets the [metrics.Meter] for recording rate limiter metrics.
func (b *TokenBucketLimiterBuilder) UseMeter(v metrics.Meter) *TokenBucketLimiterBuilder {
	b.collector = v
	return b
}

// UseClientService sets the [tokenbucket.ClientService] used to resolve
// per-client rate limits from authenticated tokens. When unset, the limiter
// falls back to IP-based rules only.
func (b *TokenBucketLimiterBuilder) UseClientService(v tokenbucket.ClientService) *TokenBucketLimiterBuilder {
	b.clientService = v
	return b
}

// RegisteredStorageTypes returns a snapshot of the names registered with UseStorage for standalone configuration validation.
func (b *TokenBucketLimiterBuilder) RegisteredStorageTypes() []storageconfig.BackendType {
	return slices.Sorted(maps.Keys(b.storages))
}
