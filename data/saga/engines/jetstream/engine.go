// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package jetstream

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/core/types/nilcheck"
	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/observability/metrics"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreretry "github.com/altessa-s/go-atlas/core/retry"
	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
)

// Sentinel errors returned by the engine.
var (
	// ErrInvalidSubject is returned by [New] when the definition name or the
	// subject prefix cannot form a literal NATS subject.
	ErrInvalidSubject = errors.New("saga engine: invalid command subject")
	// ErrIncompatibleStream is returned by [New] when the existing stream does
	// not use work-queue retention or does not cover the command subject.
	ErrIncompatibleStream = errors.New("saga engine: incompatible JetStream stream")
	// ErrIncompatibleConsumer is returned by [New] when the existing durable
	// consumer filters a different subject or does not use explicit acks.
	ErrIncompatibleConsumer = errors.New("saga engine: incompatible JetStream consumer")
	// ErrInvalidID is returned by [Engine.Submit] for a saga ID that is not
	// valid UTF-8: the durable stores persist instances as JSON or BSON, which
	// would rewrite such an ID and break resuming the instance.
	ErrInvalidID = errors.New("saga engine: saga id is not valid UTF-8")
	// ErrAlreadyRunning is returned by [Engine.Run] while another Run of the
	// same engine is active.
	ErrAlreadyRunning = errors.New("saga engine: already running")
	// ErrConsumeStopped is returned by [Engine.Run] when consumption ended
	// although its context was still live (for example, the NATS connection
	// closed). The cause is joined to it.
	ErrConsumeStopped = errors.New("saga engine: consumption stopped unexpectedly")
)

// retryJitter spreads redelivery delays so commands interrupted together do
// not come back as a wave.
const retryJitter = 0.2

// heartbeatDivisor sets the in-progress heartbeat to a fraction of AckWait,
// leaving room for two missed beats before the server redelivers.
const heartbeatDivisor = 3

// minHeartbeat floors the heartbeat interval. A consumer with an AckWait so
// short that a third of it rounds to (near) zero still gets a valid ticker; it
// just cannot be kept alive, and redeliveries are harmless.
const minHeartbeat = time.Millisecond

// fetchWait bounds one fetch request; an empty window just starts the next
// one. Shutdown cancels a pending fetch at once.
const fetchWait = 5 * time.Second

// infoTimeout bounds the consumer existence check after an empty fetch.
const infoTimeout = 2 * time.Second

// noRepanic converts a panic in an execution goroutine into a redelivery
// instead of crashing the process. Read-only after initialization.
var noRepanic = panics.NewHandleOpts().SetReallyPanic(false)

// Engine drives saga executions from a JetStream work queue. [Engine.Submit]
// durably enqueues a start command; [Engine.Run] consumes commands and drives
// each through the bound [saga.Orchestrator] until it reaches a terminal state.
// Several processes running engines for the same definition share one durable
// consumer and so compete for commands, which distributes execution.
//
// Delivery is at-least-once. A redelivered command is harmless because
// [saga.Orchestrator.Start] resumes an existing instance instead of starting a
// new one. Engine is safe for concurrent use.
type Engine[T any] struct {
	js       jetstream.JetStream
	orch     *saga.Orchestrator[T]
	consumer jetstream.Consumer

	definition  string
	subject     string
	concurrency int
	heartbeat   time.Duration
	retryDelay  coreretry.NextDelayFunc

	// fetchRetryDelay paces fetches after a transient fetch error.
	fetchRetryDelay time.Duration
	// fetchBatch and fetchWindow honor the consumer's pull-request limits
	// (MaxRequestBatch, MaxRequestExpires); fetchBatch never exceeds
	// concurrency.
	fetchBatch  int
	fetchWindow time.Duration

	serializer serializer.Serializer
	logger     *slog.Logger
	metrics    *engineMetrics

	running atomic.Bool
}

// New binds an engine to orch, creating the command stream and the durable
// consumer when they do not exist. Existing resources are validated, never
// modified: an existing consumer's AckWait sets the heartbeat interval.
//
// Example:
//
//	engine, err := sagajs.New(ctx, js, orch, sagajs.WithConcurrency(8))
//	go func() { _ = engine.Run(ctx) }()
//	err = engine.Submit(ctx, orderID, Order{ID: orderID})
func New[T any](ctx context.Context, js jetstream.JetStream, orch *saga.Orchestrator[T], opts ...Option) (*Engine[T], error) {
	if nilcheck.IsNil(js) {
		return nil, errors.New("saga engine: JetStream is required")
	}
	if orch == nil {
		return nil, errors.New("saga engine: orchestrator is required")
	}

	o := newOptions(opts...)
	definition := orch.Definition().Name()
	if !validToken(definition) || !validSubject(o.subjectPrefix) {
		return nil, fmt.Errorf("%w: prefix %q, definition %q", ErrInvalidSubject, o.subjectPrefix, definition)
	}
	if o.serializer == nil {
		o.serializer = &serializer.JSON{}
	}

	e := &Engine[T]{
		js:          js,
		orch:        orch,
		definition:  definition,
		subject:     o.subjectPrefix + "." + definition,
		concurrency: o.concurrency,
		retryDelay: coreretry.Exponential(coreretry.ExponentialConfig{
			BaseDelay: o.retryBaseDelay,
			MaxDelay:  o.retryMaxDelay,
			Jitter:    retryJitter,
		}),
		fetchRetryDelay: o.retryBaseDelay,
		serializer:      o.serializer,
		logger:          cmp.Or(o.logger, slog.New(slog.DiscardHandler)),
		metrics:         newEngineMetrics(o.collector),
	}

	stream, err := e.provisionStream(ctx, o)
	if err != nil {
		return nil, err
	}
	if e.consumer, err = e.provisionConsumer(ctx, stream, o); err != nil {
		return nil, err
	}
	e.adoptLimits(e.consumer.CachedInfo().Config)
	return e, nil
}

// provisionStream creates the stream when absent and validates it otherwise.
func (e *Engine[T]) provisionStream(ctx context.Context, o *options) (jetstream.Stream, error) {
	stream, err := e.js.Stream(ctx, o.stream)
	if errors.Is(err, jetstream.ErrStreamNotFound) {
		stream, err = e.js.CreateStream(ctx, jetstream.StreamConfig{
			Name:       o.stream,
			Subjects:   []string{o.subjectPrefix + ".>"},
			Retention:  jetstream.WorkQueuePolicy,
			Storage:    jetstream.FileStorage,
			Duplicates: o.duplicateWindow,
			MaxAge:     o.maxAge,
		})
		if errors.Is(err, jetstream.ErrStreamNameAlreadyInUse) {
			stream, err = e.js.Stream(ctx, o.stream) // Lost a creation race.
		}
	}
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "provision saga command stream")
	}

	// Beyond retention and subject coverage, a stream must store a command
	// under the subject it was published to (no transform), acknowledge
	// publishes (Submit waits for the ack), and accept publishes at all (no
	// mirror).
	cfg := stream.CachedInfo().Config
	if cfg.Retention != jetstream.WorkQueuePolicy ||
		!slices.ContainsFunc(cfg.Subjects, func(p string) bool { return subjectCovers(p, e.subject) }) ||
		cfg.SubjectTransform != nil || cfg.NoAck || cfg.Mirror != nil {
		return nil, fmt.Errorf("%w: stream %q must be an acknowledged, untransformed work queue covering %q",
			ErrIncompatibleStream, o.stream, e.subject)
	}
	return stream, nil
}

// provisionConsumer creates the durable consumer when absent and validates it
// otherwise. A concurrent creator with a different configuration surfaces as
// ErrConsumerExists and is handled like a pre-existing consumer.
func (e *Engine[T]) provisionConsumer(ctx context.Context, stream jetstream.Stream, o *options) (jetstream.Consumer, error) {
	durable := cmp.Or(o.consumer, "saga-"+e.definition)
	consumer, err := stream.Consumer(ctx, durable)
	if errors.Is(err, jetstream.ErrConsumerNotFound) {
		consumer, err = stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
			Durable:       durable,
			FilterSubject: e.subject,
			AckPolicy:     jetstream.AckExplicitPolicy,
			AckWait:       o.ackWait,
			MaxDeliver:    o.maxDeliver,
			MaxAckPending: e.concurrency * 2, //nolint:mnd // one buffered command per running slot
		})
		if errors.Is(err, jetstream.ErrConsumerExists) {
			consumer, err = stream.Consumer(ctx, durable)
		}
	}
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "provision saga command consumer")
	}

	cfg := consumer.CachedInfo().Config
	filtered := cfg.FilterSubject == e.subject ||
		(cfg.FilterSubject == "" && len(cfg.FilterSubjects) == 1 && cfg.FilterSubjects[0] == e.subject)
	// A headers-only consumer would deliver commands without their body; the
	// engine would terminate every one of them as undecodable.
	if !filtered || cfg.AckPolicy != jetstream.AckExplicitPolicy || cfg.AckWait <= 0 || cfg.HeadersOnly {
		return nil, fmt.Errorf("%w: consumer %q must filter %q with explicit acks and full payloads", ErrIncompatibleConsumer, durable, e.subject)
	}
	return consumer, nil
}

// adoptLimits derives the heartbeat interval and the pull-request shape from
// the consumer's actual configuration, which may predate this engine.
func (e *Engine[T]) adoptLimits(cfg jetstream.ConsumerConfig) {
	// With BackOff the server uses BackOff[0] for the first delivery and later
	// entries for redeliveries, so the shortest positive deadline governs.
	deadline := cfg.AckWait
	for _, b := range cfg.BackOff {
		if b > 0 && b < deadline {
			deadline = b
		}
	}
	e.heartbeat = max(deadline/heartbeatDivisor, minHeartbeat)
	e.fetchBatch = e.concurrency
	if cfg.MaxRequestBatch > 0 {
		e.fetchBatch = min(e.fetchBatch, cfg.MaxRequestBatch)
	}
	e.fetchWindow = fetchWait
	if cfg.MaxRequestExpires > 0 {
		e.fetchWindow = min(fetchWait, cfg.MaxRequestExpires)
	}
}

// Submit durably enqueues a start command for saga id with initial data and
// returns once JetStream acknowledged the publish. Submitting the same id again
// within the stream's duplicate window is collapsed by JetStream; a later
// duplicate is harmless because Start is idempotent on the saga ID.
func (e *Engine[T]) Submit(ctx context.Context, id string, data T) error {
	if id == "" {
		return sagaerrs.ErrEmptyID
	}
	if !utf8.ValidString(id) {
		return ErrInvalidID
	}
	payload, err := e.serializer.Serialize(&data)
	if err != nil {
		return coreerrs.WrapOperation(err, "serialize saga data")
	}
	body, err := encodeCommand(id, payload)
	if err != nil {
		return coreerrs.WrapOperation(err, "encode saga command")
	}
	if _, err := e.js.Publish(ctx, e.subject, body, jetstream.WithMsgID(msgID(e.definition, id))); err != nil {
		return coreerrs.WrapOperation(err, "publish saga command")
	}
	e.metrics.submitted.Inc()
	return nil
}

// Run consumes start commands and drives each through the orchestrator, at most
// WithConcurrency at a time, until ctx is done. It then stops fetching,
// cancels and waits for in-flight executions, and returns nil. Interrupted
// executions are returned to the queue for another engine or a later Run.
//
// Commands are fetched only into free execution slots, so a fetched command
// always starts at once and is kept alive by heartbeats. If consumption ends
// while ctx is still live, Run cancels and joins in-flight work and returns an
// error wrapping [ErrConsumeStopped]; the caller may call Run again. A closed
// connection is noticed immediately. A deleted consumer is noticed by a pending
// fetch or by the existence check after an empty fetch; Run then cancels and
// joins in-flight executions like on any unexpected stop, leaving their
// instances non-terminal for a redelivery or the recovery cycle to resume.
// While every slot is busy no fetch is pending, so the deletion is noticed
// only once a slot frees.
func (e *Engine[T]) Run(ctx context.Context) error {
	if !e.running.CompareAndSwap(false, true) {
		return ErrAlreadyRunning
	}
	defer e.running.Store(false)

	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// A closed connection cancels everything at once, wherever the loop is:
	// waiting for a slot, inside a fetch, or backing off after an error.
	var connLost atomic.Bool
	if nc := e.js.Conn(); nc != nil {
		closed := nc.StatusChanged(nats.CLOSED)
		defer nc.RemoveStatusListener(closed)
		if nc.IsClosed() {
			return errors.Join(ErrConsumeStopped, nats.ErrConnectionClosed)
		}
		watcherDone := make(chan struct{})
		defer func() { <-watcherDone }()
		go func() {
			defer close(watcherDone)
			select {
			case <-closed:
				connLost.Store(true)
				cancel()
			case <-execCtx.Done():
			}
		}()
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, e.concurrency)
	err := e.consume(execCtx, sem, &wg)
	cancel()
	wg.Wait()
	if connLost.Load() {
		return errors.Join(ErrConsumeStopped, nats.ErrConnectionClosed)
	}
	return err
}

// consume reserves free execution slots, fetches at most that many commands
// and starts one execution per command, until ctx is done or consumption
// cannot continue. core/runtime/concurrency does not fit: it fans out over a
// finite slice, whereas this is an unbounded queue with per-message acks.
func (e *Engine[T]) consume(ctx context.Context, sem chan struct{}, wg *sync.WaitGroup) error {
	for {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			return nil
		}
		reserved := 1 + reserveFree(sem, e.fetchBatch-1)

		started, err := e.fetch(ctx, reserved, sem, wg)
		for range reserved - started {
			<-sem
		}
		// A consumer deleted while no pull request was outstanding produces no
		// deletion notice: later fetches just find no responder or stay empty.
		// Confirm the consumer still exists whenever a fetch yields nothing.
		if started == 0 && ctx.Err() == nil {
			if gone := e.consumerGone(ctx); gone != nil {
				return errors.Join(ErrConsumeStopped, gone)
			}
		}
		switch {
		case err == nil:
		case ctx.Err() != nil:
			return nil
		case isTerminalConsumeError(err):
			return errors.Join(ErrConsumeStopped, err)
		default:
			e.logger.Warn("saga engine: fetch failed; retrying", slog.Any("error", err))
			select {
			case <-time.After(e.fetchRetryDelay):
			case <-ctx.Done():
				return nil
			}
		}
	}
}

// consumerGone reports the error when the consumer (or its stream) is
// confirmed missing, and nil otherwise — including when the lookup itself
// fails transiently.
func (e *Engine[T]) consumerGone(ctx context.Context) error {
	ictx, cancel := context.WithTimeout(ctx, infoTimeout)
	defer cancel()
	_, err := e.consumer.Info(ictx)
	if errors.Is(err, jetstream.ErrConsumerNotFound) || errors.Is(err, jetstream.ErrStreamNotFound) {
		return err
	}
	return nil
}

// reserveFree takes up to limit currently free slots without blocking.
func reserveFree(sem chan struct{}, limit int) int {
	for n := range limit {
		select {
		case sem <- struct{}{}:
		default:
			return n
		}
	}
	return limit
}

// fetch pulls up to limit commands and starts an execution for each; every
// started execution releases one reserved slot when it ends. It reports how
// many executions it started. An idle fetch window is not an error.
func (e *Engine[T]) fetch(ctx context.Context, limit int, sem chan struct{}, wg *sync.WaitGroup) (int, error) {
	fctx, cancel := context.WithTimeout(ctx, e.fetchWindow)
	defer cancel()
	batch, err := e.consumer.Fetch(limit, jetstream.FetchContext(fctx))
	if err != nil {
		return 0, idle(fctx, err)
	}
	started := 0
	for msg := range batch.Messages() {
		started++
		wg.Go(func() {
			defer func() { <-sem }()
			e.execute(ctx, msg)
		})
	}
	return started, idle(fctx, batch.Error())
}

// idle maps the end of an empty fetch window to nil.
func idle(fctx context.Context, err error) error {
	if err == nil || (errors.Is(err, context.DeadlineExceeded) && errors.Is(fctx.Err(), context.DeadlineExceeded)) ||
		errors.Is(err, jetstream.ErrNoMessages) || errors.Is(err, nats.ErrTimeout) {
		return nil
	}
	return err
}

// execute decodes one command, drives its saga and settles the message.
func (e *Engine[T]) execute(ctx context.Context, msg jetstream.Msg) {
	e.metrics.consumed.Inc()
	// Installed first so a panicking serializer or T.UnmarshalJSON is covered
	// too; the command comes back after the usual redelivery delay.
	defer panics.HandleWithOpts(ctx, noRepanic, func(_ context.Context, r any) {
		e.logger.Error("saga engine: execution panicked", slog.String("subject", msg.Subject()), slog.Any("panic", r))
		e.nak(msg, e.redeliveryDelay(msg))
	})
	// Heartbeats cover decoding too: a slow serializer must not let the
	// command be redelivered to another engine meanwhile.
	stop := e.keepAlive(msg)
	defer stop()

	cmd, err := decodeCommand(msg.Data())
	if err != nil {
		e.logger.Error("saga engine: terminating undecodable command", slog.String("subject", msg.Subject()), slog.Any("error", err))
		e.settle(msg, msg.Term, e.metrics.terminated, "term")
		return
	}
	// Always decode, even zero bytes: the serializer decides what an empty
	// representation means, and the default JSON one rejects it.
	var data T
	if decodeErr := e.serializer.Deserialize(cmd.Data, &data); decodeErr != nil {
		e.logger.Error("saga engine: terminating command with undecodable data", slog.String("id", cmd.ID), slog.Any("error", decodeErr))
		e.settle(msg, msg.Term, e.metrics.terminated, "term")
		return
	}

	e.metrics.inFlight.Inc()
	defer e.metrics.inFlight.Dec()

	inst, err := e.orch.Start(ctx, cmd.ID, data)
	switch {
	case inst != nil && inst.Status.IsTerminal():
		e.settle(msg, msg.Ack, e.metrics.acked, "ack")
	case errors.Is(err, sagaerrs.ErrDefinitionNotFound), errors.Is(err, jetstream.ErrInvalidKey):
		// The ID belongs to another definition, or the NATS KV store cannot
		// hold it as a key: retrying can never succeed.
		e.logger.Error("saga engine: terminating command the store cannot run", slog.String("id", cmd.ID), slog.Any("error", err))
		e.settle(msg, msg.Term, e.metrics.terminated, "term")
	case ctx.Err() != nil:
		// Shutdown: hand the command to another engine, but only once this
		// engine's abandoned pull request has expired on the server; an
		// immediate Nak could be redelivered into that dead request and sit
		// there until AckWait.
		e.nak(msg, e.fetchWindow)
	default:
		delay := e.redeliveryDelay(msg)
		e.logger.Warn("saga engine: execution interrupted; redelivering",
			slog.String("id", cmd.ID), slog.Duration("delay", delay), slog.Any("error", err))
		e.nak(msg, delay)
	}
}

// settle applies one acknowledgement and counts it in done on success. A
// failed acknowledgement only delays the command: the server redelivers it
// after AckWait.
func (e *Engine[T]) settle(msg jetstream.Msg, ack func() error, done metrics.Counter, kind string) {
	if err := ack(); err != nil {
		e.metrics.ackErrors.Inc()
		e.logger.Warn("saga engine: acknowledgement failed", slog.String("kind", kind), slog.String("subject", msg.Subject()), slog.Any("error", err))
		return
	}
	done.Inc()
}

// nak returns msg for redelivery after delay.
func (e *Engine[T]) nak(msg jetstream.Msg, delay time.Duration) {
	e.settle(msg, func() error { return msg.NakWithDelay(delay) }, e.metrics.naked, "nak")
}

// keepAlive sends in-progress heartbeats until the returned stop is called, so
// a saga running longer than AckWait is not redelivered to another engine. Stop
// waits for the heartbeat goroutine to exit.
func (e *Engine[T]) keepAlive(msg jetstream.Msg) (stop func()) {
	done, exited := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(exited)
		ticker := time.NewTicker(e.heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := msg.InProgress(); err != nil {
					e.metrics.ackErrors.Inc()
					e.logger.Debug("saga engine: in-progress heartbeat failed", slog.Any("error", err))
				}
			}
		}
	}()
	return func() {
		close(done)
		<-exited
	}
}

// redeliveryDelay grows with the number of deliveries of msg.
func (e *Engine[T]) redeliveryDelay(msg jetstream.Msg) time.Duration {
	attempt := 0
	if md, err := msg.Metadata(); err == nil && md.NumDelivered > 0 {
		attempt = int(min(md.NumDelivered-1, 1<<16)) //nolint:gosec,mnd // bounded above
	}
	return e.retryDelay(attempt, nil)
}

// isTerminalConsumeError reports errors after which no further commands can be
// fetched. A bad request is a consumer/request mismatch that retrying cannot fix.
func isTerminalConsumeError(err error) bool {
	return errors.Is(err, jetstream.ErrConnectionClosed) ||
		errors.Is(err, jetstream.ErrBadRequest) ||
		errors.Is(err, nats.ErrConnectionClosed) ||
		errors.Is(err, jetstream.ErrConsumerDeleted) ||
		errors.Is(err, jetstream.ErrConsumerNotFound)
}

// validToken reports whether s is one literal NATS subject token.
func validToken(s string) bool {
	return s != "" && !strings.ContainsAny(s, ".*> \t\r\n")
}

// validSubject reports whether s is a literal NATS subject (no wildcards).
func validSubject(s string) bool {
	for token := range strings.SplitSeq(s, ".") {
		if !validToken(token) {
			return false
		}
	}
	return true
}

// subjectCovers reports whether the subject filter pattern (with * and >
// wildcards) matches the literal subject.
func subjectCovers(pattern, subject string) bool {
	pt, st := strings.Split(pattern, "."), strings.Split(subject, ".")
	for i, p := range pt {
		switch {
		case p == ">":
			return len(st) > i
		case i >= len(st):
			return false
		case p != "*" && p != st[i]:
			return false
		}
	}
	return len(pt) == len(st)
}
