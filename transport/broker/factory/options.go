// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"log/slog"

	"github.com/altessa-s/go-atlas/transport/broker"

	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
)

// UseLogger sets the logger for the builder and all created components.
func (b *BrokerBuilder) UseLogger(v *slog.Logger) *BrokerBuilder {
	b.SetLogger(v)
	return b
}

// UseDefaultLogger sets the logger to [slog.Default].
func (b *BrokerBuilder) UseDefaultLogger() *BrokerBuilder {
	return b.UseLogger(slog.Default())
}

// UseScheduler sets the task scheduler for background processes.
func (b *BrokerBuilder) UseScheduler(v corescheduler.TaskRegistrar) *BrokerBuilder {
	b.scheduler = v
	return b
}

// UsePublishConverter sets the converter for publish operations.
func (b *BrokerBuilder) UsePublishConverter(v broker.PublishConverter) *BrokerBuilder {
	b.publishConverter = v
	return b
}
