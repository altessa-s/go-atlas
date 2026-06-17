// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package saga

import (
	"context"
	"slices"
	"time"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	sagaerrs "github.com/altessa-s/go-atlas/data/saga/errs"
)

// StepFunc is a forward action of a saga step. It receives the shared, typed
// saga data by pointer so it can read inputs and record outputs for later
// steps and compensations. Returning a non-nil error fails the step (subject
// to retries); a panic is recovered and treated as a failure.
type StepFunc[T any] func(ctx context.Context, data *T) error

// CompensateFunc undoes the effect of a previously completed [StepFunc]. It
// must be idempotent: the orchestrator may invoke it more than once across
// retries and crash recovery. A nil compensation marks the step as having
// nothing to undo (a read-only or naturally-idempotent step).
type CompensateFunc[T any] func(ctx context.Context, data *T) error

// RetryPolicy bounds and paces the retries of a single step action or
// compensation. It is a plain value type (not functional options) because it
// is small, copied by value, and frequently shared between steps.
type RetryPolicy struct {
	// MaxAttempts is the total number of invocations, including the first.
	// A value <= 1 disables retries (a single attempt). MaxAttempts=3 means
	// the action is called up to three times before giving up.
	MaxAttempts int
	// BaseDelay is the first backoff delay; subsequent delays grow
	// exponentially (factor 1.5) up to MaxDelay. It must be > 0 for retries to
	// occur: a zero BaseDelay yields a zero delay, which stops retrying after
	// the first attempt (so MaxAttempts has no effect). Use a small positive
	// value for near-immediate retries.
	BaseDelay time.Duration
	// MaxDelay caps the exponential backoff. Zero means unbounded.
	MaxDelay time.Duration
}

// Step is a single unit of work in a [Definition]: a forward action plus an
// optional compensation and per-step overrides. Sequential steps are added via
// Builder.Step; parallel-group members are built with [NewStep].
type Step[T any] struct {
	name         string
	action       StepFunc[T]
	compensation CompensateFunc[T]
	pivot        bool
	readOnly     bool
	timeout      time.Duration // 0 → orchestrator default
	retry        *RetryPolicy  // nil → orchestrator default
}

// Name returns the step's name.
func (s Step[T]) Name() string { return s.name }

// stage is one position in a saga: a single step, or a parallel group of
// steps that run (and compensate) concurrently.
type stage[T any] struct {
	name     string
	steps    []Step[T]
	parallel bool
}

func (st stage[T]) hasPivot() bool {
	for i := range st.steps {
		if st.steps[i].pivot {
			return true
		}
	}
	return false
}

// Definition is an immutable, validated saga blueprint: an ordered list of
// stages plus the compensation policy. Build one with [NewDefinition] and the
// fluent builder, then hand it to [New] to create an orchestrator.
type Definition[T any] struct {
	name     string
	stages   []stage[T]
	pivotIdx int      // index of the first stage containing the pivot; len(stages) if none
	missing  []string // names of compensatable steps (before pivot) lacking a compensation
	policy   Policy
}

// Name returns the definition name. It is stored on every [Instance] and must
// match when resuming.
func (d *Definition[T]) Name() string { return d.name }

// Builder accumulates stages for a [Definition]. It is not safe for concurrent
// use; build a definition once at startup and share the result.
type Builder[T any] struct {
	name   string
	stages []stage[T]
}

// NewDefinition starts building a saga definition with the given name.
func NewDefinition[T any](name string) *Builder[T] {
	return &Builder[T]{name: name}
}

// Step appends a single sequential step and returns a [StepBuilder] for
// configuring it (compensation, pivot, timeout, retries) before continuing the
// definition chain.
func (b *Builder[T]) Step(name string, action StepFunc[T]) *StepBuilder[T] {
	b.stages = append(b.stages, stage[T]{name: name, steps: []Step[T]{{name: name, action: action}}})
	return &StepBuilder[T]{Builder: b}
}

// Parallel appends a stage whose steps run concurrently. On forward execution
// every step in the group must succeed for the stage to commit; on rollback
// their compensations run concurrently. Build members with [NewStep].
func (b *Builder[T]) Parallel(name string, specs ...*StepSpec[T]) *Builder[T] {
	steps := make([]Step[T], len(specs))
	for i, sp := range specs {
		steps[i] = sp.step
	}
	b.stages = append(b.stages, stage[T]{name: name, steps: steps, parallel: true})
	return b
}

// Build validates the accumulated stages and returns an immutable
// [Definition]. With [PolicyEnforce] it returns [errs.ErrNoCompensation] when a
// compensatable step (before the pivot, not marked read-only) lacks a
// compensation. It returns [errs.ErrEmptyDefinition] when no steps were added.
func (b *Builder[T]) Build(opts ...BuildOption) (*Definition[T], error) {
	cfg := buildOptions(opts...)

	if len(b.stages) == 0 {
		return nil, sagaerrs.ErrEmptyDefinition
	}

	pivotIdx := len(b.stages)
	for i := range b.stages {
		if b.stages[i].hasPivot() {
			pivotIdx = i
			break
		}
	}

	// Compensatable region = stages strictly before the pivot stage.
	var missing []string
	for i := 0; i < pivotIdx; i++ {
		for j := range b.stages[i].steps {
			s := b.stages[i].steps[j]
			missing = coreslices.AppendIf(missing, s.compensation == nil && !s.readOnly, s.name)
		}
	}

	if cfg.policy == PolicyEnforce && len(missing) > 0 {
		return nil, coreerrs.Wrapf(sagaerrs.ErrNoCompensation, "steps %v", missing)
	}

	return &Definition[T]{
		name:     b.name,
		stages:   slicesCloneStages(b.stages),
		pivotIdx: pivotIdx,
		missing:  missing,
		policy:   cfg.policy,
	}, nil
}

// MustBuild is like [Builder.Build] but panics on a validation error. Use it
// for definitions wired at package initialization where a malformed saga is a
// programming error.
func (b *Builder[T]) MustBuild(opts ...BuildOption) *Definition[T] {
	d, err := b.Build(opts...)
	if err != nil {
		panic(err)
	}
	return d
}

// StepBuilder configures the most recently added sequential step. Its methods
// return the builder so configuration and the next Step/Parallel/Build call
// chain fluently. Because it is a method receiver, the type parameter is fixed
// and never needs to be written at the call site.
type StepBuilder[T any] struct {
	*Builder[T]
}

func (sb *StepBuilder[T]) last() *Step[T] {
	st := &sb.stages[len(sb.stages)-1]
	return &st.steps[0]
}

// Compensate attaches a compensation to the step.
func (sb *StepBuilder[T]) Compensate(fn CompensateFunc[T]) *StepBuilder[T] {
	sb.last().compensation = fn
	return sb
}

// Pivot marks the step as the saga's pivot (point of no return). Failures at or
// after the pivot stage roll forward (retry) instead of compensating. There
// should be at most one pivot.
func (sb *StepBuilder[T]) Pivot() *StepBuilder[T] {
	sb.last().pivot = true
	return sb
}

// ReadOnly marks a step that intentionally has no compensation, suppressing the
// compensation-policy warning for it. See [WithCompensationPolicy].
func (sb *StepBuilder[T]) ReadOnly() *StepBuilder[T] {
	sb.last().readOnly = true
	return sb
}

// Timeout overrides the orchestrator's default per-step timeout for this step.
func (sb *StepBuilder[T]) Timeout(d time.Duration) *StepBuilder[T] {
	if d > 0 {
		sb.last().timeout = d
	}
	return sb
}

// Retry overrides the orchestrator's default retry policy for this step using
// decomposed parameters. It pairs with [StepBuilder.RetryPolicy].
func (sb *StepBuilder[T]) Retry(maxAttempts int, base, maxDelay time.Duration) *StepBuilder[T] {
	return sb.RetryPolicy(RetryPolicy{MaxAttempts: maxAttempts, BaseDelay: base, MaxDelay: maxDelay})
}

// RetryPolicy overrides the orchestrator's default retry policy for this step
// with an explicit [RetryPolicy].
func (sb *StepBuilder[T]) RetryPolicy(p RetryPolicy) *StepBuilder[T] {
	pc := p
	sb.last().retry = &pc
	return sb
}

// StepSpec is a parallel-group member under construction. Build it with
// [NewStep] and configure it with the chainable methods, then pass it to
// Builder.Parallel.
type StepSpec[T any] struct {
	step Step[T]
}

// NewStep starts building a parallel-group step from a name and forward action.
func NewStep[T any](name string, action StepFunc[T]) *StepSpec[T] {
	return &StepSpec[T]{step: Step[T]{name: name, action: action}}
}

// Compensate attaches a compensation to the parallel step.
func (sp *StepSpec[T]) Compensate(fn CompensateFunc[T]) *StepSpec[T] {
	sp.step.compensation = fn
	return sp
}

// Pivot marks the parallel step as the saga's pivot.
func (sp *StepSpec[T]) Pivot() *StepSpec[T] {
	sp.step.pivot = true
	return sp
}

// ReadOnly marks a parallel step that intentionally has no compensation.
func (sp *StepSpec[T]) ReadOnly() *StepSpec[T] {
	sp.step.readOnly = true
	return sp
}

// Timeout overrides the per-step timeout for the parallel step.
func (sp *StepSpec[T]) Timeout(d time.Duration) *StepSpec[T] {
	if d > 0 {
		sp.step.timeout = d
	}
	return sp
}

// Retry overrides the retry policy for the parallel step.
func (sp *StepSpec[T]) Retry(maxAttempts int, base, maxDelay time.Duration) *StepSpec[T] {
	return sp.RetryPolicy(RetryPolicy{MaxAttempts: maxAttempts, BaseDelay: base, MaxDelay: maxDelay})
}

// RetryPolicy overrides the retry policy for the parallel step with an explicit
// [RetryPolicy].
func (sp *StepSpec[T]) RetryPolicy(p RetryPolicy) *StepSpec[T] {
	pc := p
	sp.step.retry = &pc
	return sp
}

// slicesCloneStages returns a shallow copy of the stage slice (and each
// stage's step slice) so the built definition is insulated from later builder
// reuse. Step structs are value types and copied by assignment.
func slicesCloneStages[T any](stages []stage[T]) []stage[T] {
	out := make([]stage[T], len(stages))
	for i := range stages {
		out[i] = stages[i]
		out[i].steps = slices.Clone(stages[i].steps)
	}
	return out
}
