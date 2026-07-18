// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package scheduler implements the gRPC SchedulerService defined in
// proto/scheduler/v1/scheduler.proto.
//
// [Handler] delegates to a [sched.Scheduler] and exposes task lifecycle
// operations (Get, List, Pause, Resume, Disable, Enable, Trigger,
// SkipNextRun) as well as scheduler status queries and task history listing.
//
// List and ListHistory accept an optional CEL filter expression; the handler
// passes it to the scheduler's paginated methods, which parse the filter and
// push evaluation to the storage backend.
//
// Scheduler domain errors are translated to appropriate gRPC status codes
// (NotFound, FailedPrecondition, InvalidArgument, Internal) by the internal
// mapError function.
//
// # Usage
//
//	handler, err := scheduler.New(sched)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	handler.Register(grpcServer, stopCh)
package scheduler
