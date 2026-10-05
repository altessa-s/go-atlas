// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package jetstream drives saga executions from a NATS JetStream work queue.
// [Engine.Submit] durably enqueues a "start saga" command; [Engine.Run]
// consumes commands and drives each through a [saga.Orchestrator] until the
// saga reaches a terminal state. Together with the NATS KeyValue store in
// data/saga/storages/nats this gives an all-NATS saga deployment.
//
// # Delivery
//
// Commands are published to <prefix>.<definition> on a work-queue stream, so
// each is consumed once. Every process running an engine for the same
// definition shares one durable consumer and competes for commands, which
// distributes execution across nodes. Delivery is at-least-once: a redelivered
// command is harmless because Orchestrator.Start resumes an existing instance.
// JetStream collapses a duplicate Submit within the stream's duplicate window.
//
// # Acknowledgement
//
//   - The saga reached a terminal state (completed, compensated, or failed and
//     dead-lettered) → Ack. A business failure is a final outcome, not a
//     delivery failure.
//   - The execution was interrupted (store outage, the instance is busy, the
//     engine shut down) → Nak with exponential backoff; the next delivery
//     resumes the instance.
//   - The command cannot be decoded or belongs to another definition → Term.
//
// Run fetches commands only into free execution slots, so a fetched command
// starts at once. Running executions send in-progress heartbeats at a third of
// the consumer's AckWait, so a saga longer than AckWait is not redelivered
// meanwhile.
//
// # Provisioning
//
// [New] creates the stream and the durable consumer when they are absent and
// only validates them otherwise; stream and consumer options apply at creation
// and an existing configuration always wins.
//
// # Usage
//
//	orch := saga.New(natsstore, def, saga.WithScheduler(sched))
//	if err := orch.RegisterRecovery(ctx); err != nil {
//		return err
//	}
//	engine, err := sagajs.New(ctx, js, orch, sagajs.WithConcurrency(8))
//	if err != nil {
//		return err
//	}
//	go func() { _ = engine.Run(ctx) }()
//
//	err = engine.Submit(ctx, orderID, Order{ID: orderID})
package jetstream
