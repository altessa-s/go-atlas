// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package inprogress provides a [Manager] that periodically sends InProgress
// heartbeats for messages being processed, preventing premature redelivery
// during long-running tasks. The manager uses [Manager.RunTickCycle] which
// should be registered with a scheduler for periodic execution.
//
// Any type implementing the [Heartbeater] interface can be registered.
// [msg.Message] satisfies this interface via [msg.Message.InProgress].
//
// Create a Manager and register its tick callback with a scheduler:
//
//	mgr, err := inprogress.New(
//	    inprogress.WithLogger(logger),
//	)
//	if err != nil {
//	    return err
//	}
//	sched.Register(ctx, schedtask.Config{
//	    ID:       "inprogress-heartbeat",
//	    Schedule: "@every 1s",
//	    Func:     mgr.RegisterTickSchedulerFunc(),
//	})
//
// Inside a message handler, register the message before long-running work:
//
//	func (h *Handler) Handle(ctx context.Context, m *msg.Message) {
//	    stop := h.inProgress.Register(m, 5*time.Second)
//	    defer stop()
//
//	    // Long-running processing...
//	    result, err := h.service.HeavyOperation(ctx, m.Data)
//	    if err != nil {
//	        _ = m.Nak()
//	        return
//	    }
//
//	    _ = m.Ack()
//	}
package inprogress
