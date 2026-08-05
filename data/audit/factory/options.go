// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas/data/audit"

	coreruntime "github.com/altessa-s/go-atlas/core/runtime"
)

// UseLogger sets the logger for the builder and all created components.
func (b *AuditorBuilder) UseLogger(v *slog.Logger) *AuditorBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *AuditorBuilder) UseDefaultLogger() *AuditorBuilder {
	return b.UseLogger(slog.Default())
}

// UseDispatcher sets the [audit.Dispatcher] that the Auditor will use for
// async event dispatch. The dispatcher must already be started.
//
// Supplying one takes the `storage` and `dispatch` configuration sections out
// of play: the builder wires what it is given rather than building its own
// engine, and does not take responsibility for stopping it.
func (b *AuditorBuilder) UseDispatcher(v audit.Dispatcher) *AuditorBuilder {
	b.dispatcher = v
	return b
}

// UseMongoDatabase sets the database backing the MongoDB audit storage.
// Required when `storage.type` is "mongo", ignored otherwise.
func (b *AuditorBuilder) UseMongoDatabase(v *mongo.Database) *AuditorBuilder {
	b.mongoDatabase = v
	return b
}

// UseShutdownHooks sets the scope the builder registers the components it owns
// into. Without it they go into the process-wide registry, which runs once for
// the whole program — pass a group when the audit subsystem must be stoppable
// on its own, e.g. when the caller may tear it down and rebuild it.
func (b *AuditorBuilder) UseShutdownHooks(v *coreruntime.HookGroup) *AuditorBuilder {
	b.shutdownHooks = v
	return b
}
