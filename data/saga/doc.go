// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package saga runs inter-service distributed transactions using the
// orchestration-based saga pattern: a sequence of local steps, each with an
// optional compensating action that undoes it. When a step fails, the
// already-completed steps are rolled back in reverse order, so the overall
// operation leaves no partial effects.
//
// The orchestrator is generic over the saga's shared data type T. It persists a
// checkpoint to a pluggable [Store] after every stage, so an instance survives
// a crash and can be resumed or automatically rolled back. State storage is
// backend-agnostic — the in-memory backend ships in storages/memory and durable
// backends plug in behind the same [Store] interface.
//
// # Model
//
//   - A [Definition] is an ordered list of stages. A stage is a single step or
//     a parallel group of steps that run (and compensate) concurrently.
//   - A step has a forward action and an optional [CompensateFunc]. A step with
//     no compensation is treated as read-only (mark it [ReadOnly] to document
//     intent and silence the compensation-policy warning).
//   - The [Pivot] marks the point of no return: failures at or after the pivot
//     stage roll forward (retry) instead of compensating.
//
// # Fault tolerance and protection
//
//   - Per-step timeouts ([WithStepTimeout]) and bounded exponential-backoff
//     retries ([WithMaxStepAttempts]); a panic in a step is recovered and
//     treated as a failure.
//   - Optimistic concurrency: [Store.Update] is compare-and-swap on a version,
//     so two coordinators cannot advance the same instance.
//   - Auto-rollback on timeout: set [WithSagaTimeout] to give each instance a
//     deadline; a background recovery cycle ([Orchestrator.RunRecoveryCycle],
//     registrable with a scheduler) rolls back instances past their deadline
//     and resumes ones left mid-flight by a crash.
//   - A dead-letter hook ([WithOnDeadLetter]) fires when a saga reaches the
//     unrecoverable [StatusFailed] state (a compensation gave up).
//
// Step actions and compensations must be idempotent: on interruption a step may
// run again when the instance is resumed.
//
// # Usage
//
//	type Order struct {
//		ID        string
//		StockHold string
//		ChargeID  string
//	}
//
//	def := saga.NewDefinition[Order]("place-order").
//		Step("reserve-stock", reserveStock).Compensate(releaseStock).
//		Step("charge-card", chargeCard).Compensate(refund).Pivot().
//		Step("ship", ship).
//		MustBuild()
//
//	store := memory.New()
//	orch := saga.New(store, def,
//		saga.WithStepTimeout(10*time.Second),
//		saga.WithSagaTimeout(5*time.Minute),
//	)
//
//	inst, err := orch.Start(ctx, order.ID, order)
//	if err != nil {
//		// inst.Status is StatusCompensated (rolled back) or StatusFailed.
//	}
package saga
