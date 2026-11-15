// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/reliability/outbox"

	schedtask "github.com/altessa-s/go-horai/task"
)

// UseLogger sets the logger for the builder and all created components.
func (b *OutboxBuilder) UseLogger(v *slog.Logger) *OutboxBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *OutboxBuilder) UseDefaultLogger() *OutboxBuilder {
	return b.UseLogger(slog.Default())
}

// UseScheduler sets the task scheduler for background processes.
func (b *OutboxBuilder) UseScheduler(v schedtask.Registrar) *OutboxBuilder {
	b.scheduler = v
	return b
}

// UseOutboxOptions adds code-supplied outbox options — a retry predicate
// ([outbox.WithShouldRetry]), a metrics collector, a compaction filter —
// applied after the options built from configuration, so they win on overlap.
func (b *OutboxBuilder) UseOutboxOptions(opts ...outbox.Option) *OutboxBuilder {
	b.outboxOpts = append(b.outboxOpts, opts...)
	return b
}
