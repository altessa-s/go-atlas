// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/altessa-s/go-atlas-core/io/wal"
	"github.com/altessa-s/go-atlas-core/runtime"
	"github.com/altessa-s/go-atlas-core/runtime/dispatch"
	"github.com/altessa-s/go-atlas/observability/metrics"
)

// Dispatcher abstracts async item dispatch. The implementation (e.g.
// [dispatch.Engine]) must be created and started before being passed
// to [New], and shut down separately by the caller.
//
// Implementations that also satisfy [Enqueuer] report typed rejection reasons
// on [Auditor.Emit]; dispatchers that only implement Submit are counted and
// logged with [DropReasonRejected].
type Dispatcher interface {
	Submit(item *Event) bool
	Dropped() int64
}

// Enqueuer is an optional capability implemented by a [Dispatcher] (such as
// [dispatch.Engine]) that reports typed admission rejections.
type Enqueuer interface {
	Enqueue(item *Event) (dispatch.Submission, error)
}

const (
	auditorStateUnstarted uint32 = iota
	auditorStateStarted
	auditorStateShutdown
)

// Auditor is the main entry point for emitting audit events. It is a thin
// facade over a [Dispatcher] with an audit-specific event schema:
// ID/timestamp auto-fill, service-info injection, and a fluent
// EventBuilder.
//
// The dispatcher is an external dependency created and managed by the
// caller — typically through [factory/audit]. Auditor uses [Enqueuer] when
// available and otherwise falls back to [Dispatcher.Submit].
type Auditor struct {
	opts                   *options
	metrics                *auditMetrics
	dispatcher             Dispatcher
	lifecycleMu            sync.Mutex
	shutdownHookOnce       sync.Once
	state                  atomic.Uint32
	notStartedDropLogged   atomic.Bool
	shutdownDropLogged     atomic.Bool
	closedEngineDropLogged atomic.Bool
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
		metrics:    newAuditMetrics(o.meter, o.metricsSubsystem),
		dispatcher: dispatcher,
	}, nil
}

// Start marks the auditor as active and registers a shutdown hook (once per
// [Auditor]) so the auditor stops accepting events on termination. Calling
// Start on an already active auditor returns [ErrAuditorAlreadyStarted].
//
// An auditor stopped via [Auditor.Shutdown] may be started again while the
// externally owned [Dispatcher] remains running. A [dispatch.Engine] is
// single-use once shut down, so when the engine itself was stopped (for example
// via a shared [runtime.HookGroup] in [factory/audit]), build a new engine and
// auditor instead.
//
// The hook goes into the process-wide registry by default, which runs once for
// the whole program — pass [WithShutdownHooks] to register into a scope that
// can be shut down on its own.
func (a *Auditor) Start() error {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()

	if a.state.Load() == auditorStateStarted {
		return ErrAuditorAlreadyStarted
	}
	a.shutdownDropLogged.Store(false)
	a.state.Store(auditorStateStarted)

	a.shutdownHookOnce.Do(func() {
		if a.opts.shutdownHooks != nil {
			a.opts.shutdownHooks.OnShutdown(a.Shutdown)
		} else {
			runtime.OnShutdown(a.Shutdown)
		}
	})

	a.opts.logger.Info("auditor started")

	return nil
}

// Shutdown stops the auditor from accepting new events without closing the
// externally owned [Dispatcher]. Calling [Auditor.Start] again re-enables
// admission over that dispatcher.
func (a *Auditor) Shutdown(_ context.Context) error {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()

	if a.state.Load() != auditorStateStarted {
		return nil
	}
	a.state.Store(auditorStateShutdown)

	a.opts.logger.Info("auditor stopped")
	return nil
}

// Emit sends an event to the async processing pipeline (fire-and-forget).
// The event ID and timestamp are set automatically if not already present.
// Returns false if the event was nil, the auditor was not active, or the
// dispatcher rejected the event.
func (a *Auditor) Emit(event *Event) bool {
	if event == nil {
		return false
	}

	switch a.state.Load() {
	case auditorStateUnstarted:
		a.recordDrop(DropReasonNotStarted)
		if a.notStartedDropLogged.CompareAndSwap(false, true) {
			a.opts.logger.Warn("audit event dropped: auditor not started",
				"reason", DropReasonNotStarted,
				"event_type", event.Type,
				"action", event.Action)
		}
		return false
	case auditorStateShutdown:
		a.recordDrop(DropReasonShutdown)
		if a.shutdownDropLogged.CompareAndSwap(false, true) {
			a.opts.logger.Warn("audit event dropped: auditor shut down",
				"reason", DropReasonShutdown,
				"event_type", event.Type,
				"action", event.Action)
		}
		return false
	}

	a.fillDefaults(event)

	if enq, ok := a.dispatcher.(Enqueuer); ok {
		if _, err := enq.Enqueue(event); err != nil {
			a.handleEnqueueError(event, err)
			return false
		}
		a.metrics.eventsEmitted.Inc()
		return true
	}

	if !a.dispatcher.Submit(event) {
		a.recordDrop(DropReasonRejected)
		a.opts.logger.Warn("audit event dropped: rejected by dispatcher",
			"reason", DropReasonRejected,
			"event_id", event.ID,
			"event_type", event.Type,
			"action", event.Action)
		return false
	}

	a.metrics.eventsEmitted.Inc()
	return true
}

func (a *Auditor) recordDrop(reason string) {
	a.metrics.eventsDropped.WithLabels(metrics.Labels{labelReason: reason}).Inc()
}

func (a *Auditor) handleEnqueueError(event *Event, err error) {
	switch {
	case errors.Is(err, dispatch.ErrEngineClosed) || errors.Is(err, wal.ErrClosed):
		a.recordDrop(DropReasonDispatcherClosed)
		if a.closedEngineDropLogged.CompareAndSwap(false, true) {
			a.opts.logger.Warn("audit event dropped: dispatcher closed",
				"reason", DropReasonDispatcherClosed,
				"event_id", event.ID,
				"event_type", event.Type,
				"action", event.Action)
		}
	case errors.Is(err, dispatch.ErrQueueFull):
		a.recordDrop(DropReasonQueueFull)
		a.opts.logger.Warn("audit event dropped: queue full",
			"reason", DropReasonQueueFull,
			"event_id", event.ID,
			"event_type", event.Type,
			"action", event.Action)
	case errors.Is(err, wal.ErrFull):
		a.recordDrop(DropReasonWALFull)
		a.opts.logger.Warn("audit event dropped: wal full",
			"reason", DropReasonWALFull,
			"event_id", event.ID,
			"event_type", event.Type,
			"action", event.Action)
	default:
		a.recordDrop(DropReasonRejected)
		a.opts.logger.Warn("audit event dropped: rejected by dispatcher",
			"reason", DropReasonRejected,
			"error_type", fmt.Sprintf("%T", err),
			"event_id", event.ID,
			"event_type", event.Type,
			"action", event.Action)
	}
}

// DroppedEvents returns the total number of events dropped by the underlying dispatcher.
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
