// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/altessa-s/go-atlas/core/runtime"
)

// Dispatcher abstracts async item dispatch. The implementation (e.g.
// [dispatch.Engine]) must be created and started before being passed
// to [New], and shut down separately by the caller.
type Dispatcher interface {
	Submit(item *Event) bool
	Dropped() int64
}

// Auditor is the main entry point for emitting audit events. It is a thin
// facade over a [Dispatcher] with an audit-specific event schema:
// ID/timestamp auto-fill, service-info injection, and a fluent
// EventBuilder.
//
// The dispatcher is an external dependency created and managed by the
// caller — typically through [data/audit/factory]. Auditor does not know
// or care how it is wired; it only calls [Dispatcher.Submit].
type Auditor struct {
	opts       *options
	metrics    *auditMetrics
	dispatcher Dispatcher
	started    atomic.Bool
}

// New creates an Auditor that dispatches events through the given
// [Dispatcher]. The dispatcher must already be started. Auditor-specific
// tunables (service info, logger) are supplied via opts.
func New(dispatcher Dispatcher, opts ...Option) (*Auditor, error) {
	if dispatcher == nil {
		return nil, ErrNilDispatcher
	}
	o := newOptions(opts...)
	return &Auditor{
		opts:       o,
		metrics:    newAuditMetrics(o.collector, o.metricsSubsystem),
		dispatcher: dispatcher,
	}, nil
}

// Start marks the auditor as active and registers a shutdown hook so the
// auditor stops accepting events on termination. The underlying [Dispatcher]
// must already be started by the caller.
//
// The hook goes into the process-wide registry by default, which runs once for
// the whole program — pass [WithShutdownHooks] to register into a scope that
// can be shut down on its own.
func (a *Auditor) Start() error {
	if !a.started.CompareAndSwap(false, true) {
		return ErrAuditorAlreadyStarted
	}

	if a.opts.shutdownHooks != nil {
		a.opts.shutdownHooks.OnShutdown(a.Shutdown)
	} else {
		runtime.OnShutdown(a.Shutdown)
	}

	a.opts.logger.Info("auditor started")

	return nil
}

// Shutdown stops the auditor from accepting new events.
func (a *Auditor) Shutdown(_ context.Context) error {
	if !a.started.CompareAndSwap(true, false) {
		return nil
	}

	a.opts.logger.Info("auditor stopped")
	return nil
}

// Emit sends an event to the async processing pipeline (fire-and-forget).
// The event ID and timestamp are set automatically if not already present.
// Returns false if the event was dropped due to a full buffer.
func (a *Auditor) Emit(event *Event) bool {
	if event == nil || !a.started.Load() {
		return false
	}

	a.fillDefaults(event)

	if !a.dispatcher.Submit(event) {
		a.metrics.eventsDropped.Inc()
		a.opts.logger.Warn("audit event dropped: buffer full",
			"event_id", event.ID,
			"event_type", event.Type,
			"action", event.Action)
		return false
	}

	a.metrics.eventsEmitted.Inc()
	return true
}

// DroppedEvents returns the total number of events dropped due to a full buffer.
func (a *Auditor) DroppedEvents() int64 {
	return a.dispatcher.Dropped()
}

// NewEvent returns a fluent EventBuilder for constructing and emitting an event.
func (a *Auditor) NewEvent(eventType EventType, action Action) *EventBuilder {
	return &EventBuilder{
		auditor: a,
		event: &Event{
			Type:   eventType,
			Action: action,
		},
	}
}

func (a *Auditor) fillDefaults(event *Event) {
	if event.ID == "" {
		event.ID = ulid.Make().String()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.Service == (ServiceInfo{}) {
		event.Service = a.opts.serviceInfo
	}
}

// StorageSink adapts a [Storage] to [dispatch.Sink] so it can be passed
// to [dispatch.NewEngine]. Exported for use by factory packages.
type StorageSink struct{ Storage Storage }

// StoreBatch implements [dispatch.Sink].
func (s StorageSink) StoreBatch(ctx context.Context, items []*Event) error {
	return s.Storage.StoreBatch(ctx, items)
}

// JSONCodec is the default [dispatch.Codec] for audit events, using JSON
// encoding. Exported for use by factory packages that construct the engine.
type JSONCodec = jsonCodec

// Logger returns the facade logger. Useful for factory packages that need
// to pass the same logger to the dispatch engine.
func (o *options) Logger() *slog.Logger { return o.logger }
