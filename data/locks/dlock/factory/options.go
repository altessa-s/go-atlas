// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/nats-io/nats.go"

	"github.com/altessa-s/go-atlas/observability/health"
)

// --- Dependency methods ---

// UseLogger sets the logger for the builder and all created components.
func (b *DLockBuilder) UseLogger(v *slog.Logger) *DLockBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *DLockBuilder) UseDefaultLogger() *DLockBuilder {
	return b.UseLogger(slog.Default())
}

// UseNatsConn sets the NATS connection used for distributed locking.
func (b *DLockBuilder) UseNatsConn(v *nats.Conn) *DLockBuilder {
	b.natsConn = v
	return b
}

// UseHealthCoordinator registers the resulting [dlock.DLock] with the
// supplied health coordinator on construction. Combine with
// [DLockBuilder.UseHealthServiceName] to override the default
// "dlock" service name when several DLock instances share a coordinator.
func (b *DLockBuilder) UseHealthCoordinator(v *health.Coordinator) *DLockBuilder {
	b.healthCoordinator = v
	return b
}

// UseHealthServiceName overrides the service name used for health
// registration. Has no effect unless [DLockBuilder.UseHealthCoordinator]
// is also called.
func (b *DLockBuilder) UseHealthServiceName(v string) *DLockBuilder {
	b.healthServiceName = v
	return b
}
