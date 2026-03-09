// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/altessa-s/go-atlas/core/runtime"

	corecontext "github.com/altessa-s/go-atlas/core/context"
)

// Auditor is the main entry point for emitting audit events.
// It manages an asynchronous dispatcher that batches and persists events
// with at-least-once delivery semantics.
type Auditor struct {
	opts       *options
	storage    Storage
	dispatcher *dispatcher
	metrics    *auditMetrics
	started    atomic.Bool
	dropped    atomic.Int64
}

// New creates a new Auditor with the given storage and options.
func New(storage Storage, opts ...Option) (*Auditor, error) {
	if storage == nil {
		return nil, ErrNilStorage
	}

	o := newOptions(opts...)

	return &Auditor{
		opts:    o,
		storage: storage,
		metrics: newAuditMetrics(o.collector),
	}, nil
}

// Start begins the asynchronous event processing pipeline.
func (a *Auditor) Start() error {
	if !a.started.CompareAndSwap(false, true) {
		return ErrAuditorAlreadyStarted
	}

	a.dispatcher = newDispatcher(a.storage, a.opts, a.metrics)
	a.dispatcher.start()

	runtime.OnShutdown(a.Shutdown)

	a.opts.logger.Info("auditor started",
		"buffer_size", a.opts.bufferSize,
		"batch_size", a.opts.batchSize,
		"workers", a.opts.workers)

	return nil
}

// Shutdown gracefully stops the auditor, draining all buffered events.
func (a *Auditor) Shutdown(ctx context.Context) error {
	if !a.started.CompareAndSwap(true, false) {
		return nil
	}

	shutdownCtx, cancel := corecontext.WithMaxTimeout(ctx, a.opts.shutdownTimeout)
	defer cancel()

	err := a.dispatcher.shutdown(shutdownCtx)

	a.opts.logger.Info("auditor stopped")
	return err
}

// Emit sends an event to the async processing pipeline (fire-and-forget).
// The event ID and timestamp are set automatically if not already present.
// Returns false if the event was dropped due to a full buffer.
func (a *Auditor) Emit(event *Event) bool {
	if event == nil || !a.started.Load() {
		return false
	}

	a.fillDefaults(event)

	if !a.dispatcher.emit(event) {
		a.dropped.Add(1)
		a.metrics.eventsDropped.Inc()
		if a.opts.onDrop != nil {
			a.opts.onDrop(event)
		}
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
	return a.dropped.Load()
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
