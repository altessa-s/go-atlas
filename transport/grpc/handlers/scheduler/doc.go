// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package scheduler implements the gRPC SchedulerService defined in
// proto/scheduler/v1/scheduler.proto over a github.com/altessa-s/go-horai
// scheduler.
//
// [Handler] delegates to a [SchedulerService] and exposes task
// lifecycle operations (Get, List, Pause, Resume, Disable, Enable, Trigger,
// SkipNextRun) as well as scheduler status queries and task history listing.
//
// go-horai leaves two things to the transport, and the handler owns them:
//
//   - Filters. List and ListHistory accept an optional CEL filter, parsed with
//     an injected go-aip filter parser and translated to a typed go-horai query:
//     &&, ||, !, comparisons and in between a field and literals, startsWith
//     and contains on a string field, and a bare boolean field.
//     [NewWithCompiler] accepts a [FilterCompiler] for a custom filter language.
//   - Page tokens. Cursors are signed with a go-aip pagination/pagetoken codec and bound to
//     the list, the task and the filter expression.
//
// Scheduler domain errors are translated to appropriate gRPC status codes
// (NotFound, FailedPrecondition, InvalidArgument, Internal) by the internal
// mapError function.
//
// # Usage
//
//	import filterparser "github.com/altessa-s/go-aip/filter/parser"
//	import pagetokenfactory "github.com/altessa-s/go-atlas/factory/ext/aip/pagination/pagetoken"
//	import scheduler "github.com/altessa-s/go-atlas/transport/grpc/handlers/scheduler"
//
//	tokens, err := pagetokenfactory.New(cfg.Scheduler.Paging).Build()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	parser, err := filterparser.New()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	handler, err := scheduler.New(sched, tokens, parser)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	handler.Register(grpcServer, stopCh)
package scheduler
