// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package eventbus

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/altessa-s/go-atlas/observability/metrics"

	slogx "github.com/altessa-s/go-atlas/observability/slog"
)

// Metric naming constants for the event bus.
const (
	metricsSubsystem = "eventbus"
	labelEvent       = "event"  // label: the event's concrete type
	labelStatus      = "status" // label: "success" or "error"
)

// observedBus decorates a Bus, recording publish latency and outcome per event
// type and flagging silent non-delivery of a required event type. Subscribe and
// the adapter registrations are forwarded unchanged via the embedded Bus; only
// Publish is instrumented.
type observedBus struct {
	Bus

	logger    *slog.Logger
	publishes metrics.Counter // labels: event, status
	latency   metrics.Timer   // labels: event
	noHandler metrics.Counter // labels: event
}

// NewObserved wraps a Bus so that every Publish records a duration and a
// success/error count labeled by event type, and—when the wrapped bus reports
// dispatch counts—increments a no-handler counter and logs a warning if a
// required event type is published with no handler. A nil collector falls back
// to a no-op collector, so the decorator is always safe to construct.
func NewObserved(bus Bus, collector metrics.Collector) Bus {
	if collector == nil {
		collector = metrics.Noop()
	}
	m := collector.WithSubsystem(metricsSubsystem)
	return &observedBus{
		Bus:    bus,
		logger: slog.Default().With(slogx.Module(metricsSubsystem)),
		publishes: m.MustCounter(metrics.MetricOpts{
			Name:       "publish_total",
			Help:       "Total number of events published through the in-process event bus.",
			LabelNames: []string{labelEvent, labelStatus},
		}),
		latency: m.MustTimer(metrics.HistogramOpts{
			MetricOpts: metrics.MetricOpts{
				Name:       "publish_duration_seconds",
				Help:       "Duration of in-process event bus publishes, including all handlers and adapters.",
				LabelNames: []string{labelEvent},
			},
			Buckets: metrics.DefaultDurationBuckets,
		}),
		noHandler: m.MustCounter(metrics.MetricOpts{
			Name:       "publish_no_handler_total",
			Help:       "Publishes of a required event type that found no handler at dispatch time.",
			LabelNames: []string{labelEvent},
		}),
	}
}

// Publish times the underlying publish, records its outcome, and—when the
// wrapped bus reports dispatch counts—flags a required event type that ran no
// handler, before returning the original result unchanged.
func (o *observedBus) Publish(ctx context.Context, event any) error {
	eventType := fmt.Sprintf("%T", event)

	stop := o.latency.WithLabels(metrics.Labels{labelEvent: eventType}).Start()
	res, err := o.publish(ctx, event)
	stop()

	status := "success"
	if err != nil {
		status = "error"
	}
	o.publishes.WithLabels(metrics.Labels{labelEvent: eventType, labelStatus: status}).Inc()

	if err == nil && res.Required && res.Handlers == 0 {
		o.noHandler.WithLabels(metrics.Labels{labelEvent: eventType}).Inc()
		o.logger.WarnContext(ctx, "required event published with no handler", slog.String(labelEvent, eventType))
	}
	return err
}

// publish dispatches through the wrapped bus, returning dispatch counts when the
// bus supports reporting them (the in-process bus does); otherwise it falls back
// to a plain Publish and reports a zero result.
func (o *observedBus) publish(ctx context.Context, event any) (dispatchResult, error) {
	if d, ok := o.Bus.(dispatcher); ok {
		return d.dispatch(ctx, event)
	}
	return dispatchResult{}, o.Bus.Publish(ctx, event)
}
