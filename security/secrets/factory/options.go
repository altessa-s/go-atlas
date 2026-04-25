// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/health"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	vaultApi "github.com/hashicorp/vault/api"
)

// --- Dependency methods ---

// UseLogger sets the logger for the builder and all created components.
func (b *ManagerBuilder) UseLogger(v *slog.Logger) *ManagerBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *ManagerBuilder) UseDefaultLogger() *ManagerBuilder {
	return b.UseLogger(slog.Default())
}

// UseScheduler sets the task scheduler for background processes.
func (b *ManagerBuilder) UseScheduler(v corescheduler.TaskRegistrar) *ManagerBuilder {
	b.scheduler = v
	return b
}

// UseHealthCoordinator sets the health coordinator for auto-registration with the health system.
func (b *ManagerBuilder) UseHealthCoordinator(v *health.Coordinator) *ManagerBuilder {
	b.healthCoordinator = v
	return b
}

// UseVaultClient sets the Vault client for the Vault secrets provider.
func (b *ManagerBuilder) UseVaultClient(v *vaultApi.Client) *ManagerBuilder {
	b.vaultClient = v
	return b
}
