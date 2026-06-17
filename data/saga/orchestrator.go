// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/core/encoding/serializer"
	"github.com/altessa-s/go-atlas/core/runtime/concurrency"
	"github.com/altessa-s/go-atlas/core/runtime/panics"
	"github.com/altessa-s/go-atlas/core/types/nilcheck"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	corecontext "github.com/altessa-s/go-atlas/core/context"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	coreretry "github.com/altessa-s/go-atlas/core/retry"
	corescheduler "github.com/altessa-s/go-atlas/core/scheduler"
	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
)

// errPanic is the sentinel wrapped around a recovered panic from a step action
// or compensation. It is internal; callers see the wrapped message.
var errPanic = errors.New("saga: panic")

// noRepanic forces panic recovery to swallow (never re-throw) the panic so a
// faulting step is always converted to an error. It is read-only after
// construction and safe to share across goroutines.
var noRepanic = panics.NewHandleOpts().SetReallyPanic(false)

// retryJitter spreads step/compensation retry delays to avoid synchronized
// retries across instances.
const retryJitter = 0.2

// Orchestrator drives a single saga [Definition] over a pluggable [Store]. It
// is safe for concurrent use: each Start/Resume operates on its own instance,
// and concurrent coordinators are serialized per instance by the store's
// optimistic-concurrency check.
type Orchestrator[T any] struct {
	store Store
	def   *Definition[T]

	logger     *slog.Logger
	serializer serializer.Serializer
	metrics    *sagaMetrics
	baseCtx    context.Context

	stepTimeout             time.Duration
	sagaTimeout             time.Duration
	defaultRetry            RetryPolicy
	maxCompensationAttempts int
	stepConcurrency         int

	scheduler         corescheduler.TaskRegistrar
	leaderElector     LeaderElector
	recoverySchedule  string
	recoveryBatchSize int
	recoveryTaskID    string

	onDeadLetter DeadLetterFunc
	shouldRetry  func(error) bool

	// defaultStepRetryOpts and compRetryOpts are the core/retry option slices
	// for the default step policy and for compensations. Both are identical on
	// every invocation, so they are built once in New and reused to keep the
	// hot path allocation-free. Steps with a per-step RetryPolicy build their
	// options on demand.
	defaultStepRetryOpts []coreretry.Option
	compRetryOpts        []coreretry.Option

	recoveryRunning             atomic.Bool
	schedulerRecoveryRegistered atomic.Bool
}

// New creates an orchestrator for def backed by store. Default settings are
// applied and can be overridden via Option values. When the definition's
// compensation policy is [PolicyWarn] (the default) and some compensatable
// steps lack a compensation, New logs a warning. When a scheduler and recovery
// schedule are configured, New registers the background recovery cycle.
func New[T any](store Store, def *Definition[T], opts ...Option) *Orchestrator[T] {
	cfg := newOptions(opts...)
	cfg.logger = cmp.Or(cfg.logger, slog.New(slog.DiscardHandler))
	cfg.baseCtx = corecontext.OrBackground(cfg.baseCtx)
	if cfg.serializer == nil {
		cfg.serializer = &serializer.JSON{}
	}

	o := &Orchestrator[T]{
		store:       store,
		def:         def,
		logger:      cfg.logger,
		serializer:  cfg.serializer,
		metrics:     newSagaMetrics(cfg.collector),
		baseCtx:     cfg.baseCtx,
		stepTimeout: cfg.stepTimeout,
		sagaTimeout: cfg.sagaTimeout,
		defaultRetry: RetryPolicy{
			MaxAttempts: cfg.maxStepAttempts,
			BaseDelay:   cfg.stepRetryBaseDelay,
			MaxDelay:    cfg.stepRetryMaxDelay,
		},
		maxCompensationAttempts: cfg.maxCompensationAttempts,
		stepConcurrency:         cfg.stepConcurrency,
		scheduler:               cfg.scheduler,
		leaderElector:           cfg.leaderElector,
		recoverySchedule:        cfg.recoverySchedule,
		recoveryBatchSize:       cfg.recoveryBatchSize,
		recoveryTaskID:          cfg.recoveryTaskID,
		onDeadLetter:            cfg.onDeadLetter,
		shouldRetry:             cfg.shouldRetry,
	}

	o.defaultStepRetryOpts = o.retryOptions(o.defaultRetry)
	o.compRetryOpts = o.retryOptions(RetryPolicy{
		MaxAttempts: o.maxCompensationAttempts,
		BaseDelay:   o.defaultRetry.BaseDelay,
		MaxDelay:    o.defaultRetry.MaxDelay,
	})

	if def.policy == PolicyWarn && len(def.missing) > 0 {
		o.logger.Warn("saga: compensatable steps lack a compensation; rollback will be incomplete",
			slog.String("definition", def.name),
			slog.Any("steps", def.missing),
			slog.String("fix", "add saga.Compensate(...), mark saga.ReadOnly(), or set WithCompensationPolicy(saga.PolicyDisabled)"),
		)
	}

	if !nilcheck.IsNil(o.scheduler) && o.recoverySchedule != "" {
		if err := o.registerRecoveryTask(); err != nil {
			o.logger.Warn("saga: failed to register recovery task", slog.Any("error", err))
		}
	}

	return o
}

// Definition returns the definition this orchestrator runs.
func (o *Orchestrator[T]) Definition() *Definition[T] { return o.def }

// Start creates a new saga instance with the given ID and initial data, then
// runs it to a terminal state, persisting a checkpoint after every stage. The
// ID doubles as an idempotency key: if an instance with the same ID already
// exists, Start resumes it instead of starting anew.
//
// Start returns the final [Instance] and:
//   - nil when the saga completed every stage;
//   - the triggering error when a step failed and the saga rolled back
//     (instance status [StatusCompensated]);
//   - an error wrapping [errs.ErrCompensationFailed] (or the post-pivot failure)
//     when the saga ended in [StatusFailed];
//   - a context or store error when execution was interrupted (the instance is
//     left non-terminal for the recovery loop or a later Resume).
//
// Step actions and compensations must be idempotent: on interruption a step may
// run again when the saga is resumed.
func (o *Orchestrator[T]) Start(ctx context.Context, id string, data T) (*Instance, error) {
	if id == "" {
		return nil, sagaerrs.ErrEmptyID
	}

	payload, err := o.serializer.Serialize(&data)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "serialize saga data")
	}

	now := time.Now().UTC()
	inst := &Instance{
		ID:         id,
		Definition: o.def.name,
		Status:     StatusRunning,
		Data:       payload,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if o.sagaTimeout > 0 {
		inst.Deadline = now.Add(o.sagaTimeout)
	}

	if err := o.store.Create(ctx, inst); err != nil {
		if errors.Is(err, sagaerrs.ErrInstanceExists) {
			return o.Resume(ctx, id)
		}
		return nil, coreerrs.WrapOperation(err, "create saga instance")
	}

	o.metrics.started.Inc()
	return o.drive(ctx, inst, &data)
}

// Resume continues a persisted instance from its checkpoint. It is used for
// crash recovery and by the background recovery cycle. It returns
// [errs.ErrInstanceNotFound] when no such instance exists,
// [errs.ErrDefinitionNotFound] when the stored definition name does not match
// this orchestrator, and [errs.ErrAlreadyTerminal] (with the loaded instance)
// when there is nothing left to run.
func (o *Orchestrator[T]) Resume(ctx context.Context, id string) (*Instance, error) {
	if id == "" {
		return nil, sagaerrs.ErrEmptyID
	}

	inst, err := o.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if inst.Definition != o.def.name {
		return nil, sagaerrs.ErrDefinitionNotFound
	}
	if inst.Status.IsTerminal() {
		return inst, sagaerrs.ErrAlreadyTerminal
	}

	var data T
	if len(inst.Data) > 0 {
		if err := o.serializer.Deserialize(inst.Data, &data); err != nil {
			return nil, coreerrs.WrapOperation(err, "deserialize saga data")
		}
	}

	return o.drive(ctx, inst, &data)
}

// drive runs the instance through the forward and/or compensation phases based
// on its current status and returns the terminal instance plus an outcome
// error (see Start).
func (o *Orchestrator[T]) drive(ctx context.Context, inst *Instance, data *T) (*Instance, error) {
	o.metrics.inFlight.Inc()
	defer o.metrics.inFlight.Dec()

	var triggerErr error
	if inst.Status == StatusRunning {
		triggerErr = o.runForward(ctx, inst, data)
		switch {
		case triggerErr == nil:
			return inst, nil // Completed.
		case inst.Status == StatusFailed:
			o.fireDeadLetter(inst)
			return inst, triggerErr
		case inst.Status != StatusCompensating:
			// Persist or context error before any transition: leave the
			// instance non-terminal for recovery, do not compensate.
			return inst, triggerErr
		}
	}

	if inst.Status == StatusCompensating {
		if err := o.compensate(ctx, inst, data); err != nil {
			if inst.Status == StatusFailed {
				o.fireDeadLetter(inst)
			}
			return inst, err
		}
		return inst, triggerErr // Compensated; surface the original cause if any.
	}

	return inst, triggerErr
}

// runForward executes stages from the current cursor. It returns nil when every
// stage commits (status set to Completed). On a pre-pivot failure it transitions
// the instance to Compensating (persisted) and returns the triggering error. On
// a post-pivot failure it transitions to Failed (persisted) and returns the
// error. On a persist/context error it leaves the status unchanged and returns
// the error.
func (o *Orchestrator[T]) runForward(ctx context.Context, inst *Instance, data *T) error {
	for inst.Stage < len(o.def.stages) {
		st := o.def.stages[inst.Stage]
		postPivot := inst.Stage >= o.def.pivotIdx

		stop := o.metrics.stageDuration.Start()
		attempts, err := o.runStage(ctx, st, data, false)
		stop()

		if err != nil {
			if coreerrs.IsContextCanceled(err) {
				return err // Transient: resume later, stays Running.
			}
			o.metrics.stepFailures.Inc()
			inst.LastError = err.Error()

			if postPivot {
				inst.Status = StatusFailed
				inst.UpdatedAt = time.Now().UTC()
				if perr := o.store.Update(ctx, inst); perr != nil {
					inst.Status = StatusRunning
					return perr
				}
				o.metrics.failed.Inc()
				return coreerrs.Wrapf(err, "post-pivot stage %q failed", st.name)
			}

			inst.Status = StatusCompensating
			inst.UpdatedAt = time.Now().UTC()
			if perr := o.store.Update(ctx, inst); perr != nil {
				inst.Status = StatusRunning
				return perr
			}
			return err
		}

		o.recordStage(inst, st, inst.Stage, StepCompleted, attempts)
		inst.Stage++
		inst.UpdatedAt = time.Now().UTC()
		if err := o.encodeInto(inst, data); err != nil {
			return err
		}
		if err := o.store.Update(ctx, inst); err != nil {
			return err
		}
		o.metrics.stepsExecuted.Add(float64(len(st.steps)))
	}

	inst.Status = StatusCompleted
	inst.UpdatedAt = time.Now().UTC()
	if err := o.store.Update(ctx, inst); err != nil {
		return err
	}
	o.metrics.completed.Inc()
	return nil
}

// compensate rolls back completed stages from the cursor down to zero, then
// marks the instance Compensated. On an unrecoverable compensation failure it
// transitions to Failed (persisted) and returns an error wrapping
// [errs.ErrCompensationFailed].
func (o *Orchestrator[T]) compensate(ctx context.Context, inst *Instance, data *T) error {
	for inst.Stage > 0 {
		st := o.def.stages[inst.Stage-1]

		attempts, err := o.runStage(ctx, st, data, true)
		if err != nil {
			if coreerrs.IsContextCanceled(err) {
				return err // Transient: resume later, stays Compensating.
			}
			inst.Status = StatusFailed
			inst.LastError = err.Error()
			inst.UpdatedAt = time.Now().UTC()
			if perr := o.store.Update(ctx, inst); perr != nil {
				inst.Status = StatusCompensating
				return perr
			}
			o.metrics.compFailures.Inc()
			o.metrics.failed.Inc()
			return coreerrs.JoinWrap(sagaerrs.ErrCompensationFailed, err)
		}

		o.recordStage(inst, st, inst.Stage-1, StepCompensated, attempts)
		inst.Stage--
		inst.UpdatedAt = time.Now().UTC()
		if err := o.encodeInto(inst, data); err != nil {
			return err
		}
		if err := o.store.Update(ctx, inst); err != nil {
			return err
		}
		o.metrics.compensations.Inc()
	}

	inst.Status = StatusCompensated
	inst.UpdatedAt = time.Now().UTC()
	if err := o.store.Update(ctx, inst); err != nil {
		return err
	}
	o.metrics.compensated.Inc()
	return nil
}

// runStage runs a stage either forward (compensating=false) or in reverse
// (compensating=true). A single-step stage runs inline; a parallel group fans
// out via core/runtime/concurrency. It returns the per-step attempt counts
// (aligned with st.steps) so recordStage can persist the real number of tries.
func (o *Orchestrator[T]) runStage(ctx context.Context, st stage[T], data *T, compensating bool) ([]int, error) {
	run := o.runStep
	if compensating {
		run = o.compensateStep
	}

	attempts := make([]int, len(st.steps))
	if len(st.steps) == 1 {
		n, err := run(ctx, st.steps[0], data)
		attempts[0] = n
		return attempts, err
	}

	copts := []concurrency.Option[int]{}
	// Forward parallel stages fail fast; compensation attempts every step.
	copts = coreslices.AppendIf(copts, !compensating, concurrency.WithStopOnError[int]())
	copts = coreslices.AppendIf(copts, o.stepConcurrency > 0, concurrency.WithConcurrency[int](o.stepConcurrency))

	// Iterate by index so each worker can record its own attempt count into a
	// distinct slot (no shared-write race).
	idx := make([]int, len(st.steps))
	for i := range idx {
		idx[i] = i
	}
	err := concurrency.Process(ctx, idx, func(ctx context.Context, i int) error {
		n, e := run(ctx, st.steps[i], data)
		attempts[i] = n
		return e
	}, copts...)
	return attempts, err
}

// runStep executes a forward step action with a per-step timeout, panic
// recovery, and bounded exponential-backoff retries. It returns the number of
// attempts made (>= 1, including the first).
func (o *Orchestrator[T]) runStep(ctx context.Context, s Step[T], data *T) (int, error) {
	timeout := o.resolveTimeout(s)

	opts := o.defaultStepRetryOpts
	if s.retry != nil {
		opts = o.retryOptions(*s.retry)
	}

	attempts := 0
	err := coreretry.Do(ctx, func(ctx context.Context) (err error) {
		attempts++
		sctx, cancel := corecontext.ApplyTimeout(ctx, timeout)
		defer cancel()
		defer panics.HandleWithOpts(sctx, noRepanic, func(_ context.Context, r any) {
			err = coreerrs.Wrapf(errPanic, "step %q panicked: %v", s.name, r)
		})
		return s.action(sctx, data)
	}, opts...)
	return attempts, err
}

// compensateStep runs a step's compensation (a no-op when the step has none)
// with a per-step timeout, panic recovery, and bounded retries. It returns the
// number of attempts made (0 when the step has no compensation).
func (o *Orchestrator[T]) compensateStep(ctx context.Context, s Step[T], data *T) (int, error) {
	if s.compensation == nil {
		return 0, nil
	}
	timeout := o.resolveTimeout(s)

	attempts := 0
	err := coreretry.Do(ctx, func(ctx context.Context) (err error) {
		attempts++
		cctx, cancel := corecontext.ApplyTimeout(ctx, timeout)
		defer cancel()
		defer panics.HandleWithOpts(cctx, noRepanic, func(_ context.Context, r any) {
			err = coreerrs.Wrapf(errPanic, "compensation %q panicked: %v", s.name, r)
		})
		return s.compensation(cctx, data)
	}, o.compRetryOpts...)
	return attempts, err
}

// retryOptions builds the core/retry options for a step or compensation from
// the resolved policy.
func (o *Orchestrator[T]) retryOptions(policy RetryPolicy) []coreretry.Option {
	return []coreretry.Option{
		coreretry.WithMaxAttempts(maxRetries(policy.MaxAttempts)),
		coreretry.WithShouldRetry(o.retryPredicate),
		coreretry.WithNextDelay(coreretry.Exponential(coreretry.ExponentialConfig{
			BaseDelay: policy.BaseDelay,
			MaxDelay:  policy.MaxDelay,
			Jitter:    retryJitter,
		})),
		coreretry.WithOnRetry(func(int, error, time.Duration) {
			o.metrics.stepRetries.Inc()
		}),
	}
}

// retryPredicate decides whether an error is retryable. Context cancellation is
// never retried; otherwise a caller-supplied predicate (WithShouldRetry) wins,
// and the default retries every error.
func (o *Orchestrator[T]) retryPredicate(err error) bool {
	if coreerrs.IsContextCanceled(err) {
		return false
	}
	if o.shouldRetry != nil {
		return o.shouldRetry(err)
	}
	return true
}

func (o *Orchestrator[T]) resolveTimeout(s Step[T]) time.Duration {
	if s.timeout > 0 {
		return s.timeout
	}
	return o.stepTimeout
}

// encodeInto serializes data into inst.Data so the persisted checkpoint
// reflects mutations made by the steps run so far.
func (o *Orchestrator[T]) encodeInto(inst *Instance, data *T) error {
	b, err := o.serializer.Serialize(data)
	if err != nil {
		return coreerrs.WrapOperation(err, "serialize saga data")
	}
	inst.Data = b
	return nil
}

// recordStage appends an observability record for each step in the stage at
// stageIdx (the actual stage being committed or compensated, which differs from
// the live cursor during rollback). attempts holds the per-step try counts
// returned by runStage, aligned with st.steps.
func (o *Orchestrator[T]) recordStage(inst *Instance, st stage[T], stageIdx int, status StepStatus, attempts []int) {
	now := time.Now().UTC()
	for i := range st.steps {
		inst.Steps = append(inst.Steps, StepRecord{
			Name:       st.steps[i].name,
			Stage:      stageIdx,
			Status:     status,
			Attempts:   attempts[i],
			FinishedAt: now,
		})
	}
}

// fireDeadLetter invokes the dead-letter hook (if any) with a clone of the
// instance, isolating the engine from a panicking or mutating hook.
func (o *Orchestrator[T]) fireDeadLetter(inst *Instance) {
	if o.onDeadLetter == nil {
		return
	}
	defer panics.HandleWithOpts(o.baseCtx, noRepanic, func(_ context.Context, r any) {
		o.logger.Error("saga: dead-letter hook panicked", slog.Any("panic", r), slog.String("id", inst.ID))
	})
	o.onDeadLetter(inst.Clone())
}

// maxRetries converts a total-attempts count (>= 1) into the zero-based maximum
// attempt index expected by core/retry (attempts 0..N). A value <= 1 yields 0
// (a single attempt, no retries).
func maxRetries(totalAttempts int) int {
	if totalAttempts <= 1 {
		return 0
	}
	return totalAttempts - 1
}
