// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/messaging/broker"
	"github.com/altessa-s/go-atlas/observability/metrics"

	brokerconfig "github.com/altessa-s/go-atlas/config/broker"
	schedtask "github.com/altessa-s/go-horai/task"
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
func (b *BrokerBuilder) UseScheduler(v schedtask.Registrar) *BrokerBuilder {
	b.scheduler = v
	return b
}

// UseMeter sets the [metrics.Meter] the broker, the provider (through
// [ProviderEnv]) and the in-progress heartbeat manager record their metrics
// on. Without one they record nothing.
func (b *BrokerBuilder) UseMeter(v metrics.Meter) *BrokerBuilder {
	b.collector = v
	return b
}

// UseProvider registers the provider function of one broker provider type
// (brokerconfig.ProviderNATS) for [BrokerBuilder.CreateProvider]. A later
// registration of the same type replaces the earlier one.
func (b *BrokerBuilder) UseProvider(typ brokerconfig.Provider, build ProviderFunc) *BrokerBuilder {
	if build == nil {
		b.errs = append(b.errs, fmt.Errorf("UseProvider %q: nil function", typ))
		return b
	}
	if b.providers == nil {
		b.providers = map[brokerconfig.Provider]ProviderFunc{}
	}
	b.providers[typ] = build
	return b
}

// UsePublishConverter sets the converter for publish operations.
func (b *BrokerBuilder) UsePublishConverter(v broker.PublishConverter) *BrokerBuilder {
	b.publishConverter = v
	return b
}
