// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/altessa-s/go-horai/task"

	"github.com/altessa-s/go-atlas/observability/health"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	secretsconfig "github.com/altessa-s/go-atlas/config/secrets"
	corefactory "github.com/altessa-s/go-atlas/factory/core"
)

// --- Dependency methods ---

// UseLogger sets the logger for the builder and all created components.
func (b *ManagerBuilder[T]) UseLogger(v *slog.Logger) *ManagerBuilder[T] {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *ManagerBuilder[T]) UseDefaultLogger() *ManagerBuilder[T] {
	return b.UseLogger(slog.Default())
}

// UseScheduler sets the task scheduler for background processes.
func (b *ManagerBuilder[T]) UseScheduler(v task.Registrar) *ManagerBuilder[T] {
	b.scheduler = v
	return b
}

// UseHealthCoordinator sets the health coordinator for auto-registration with the health system.
func (b *ManagerBuilder[T]) UseHealthCoordinator(v health.ScopedReporter) *ManagerBuilder[T] {
	b.healthCoordinator = v
	return b
}

// UseProvider registers the provider builder of one secrets provider type
// (secretsconfig.ProviderVault, ...). A later registration of the same type
// replaces the earlier one; the memory provider is built in and cannot be
// replaced.
func (b *ManagerBuilder[T]) UseProvider(typ secretsconfig.Provider, build ProviderBuilder[T]) *ManagerBuilder[T] {
	if build == nil {
		b.errs = append(b.errs, corefactory.Errorf(coreerrs.ErrInvalidArgument, "UseProvider %q: nil builder", typ))
		return b
	}
	if b.providers == nil {
		b.providers = map[secretsconfig.Provider]ProviderBuilder[T]{}
	}
	b.providers[typ] = build
	return b
}
