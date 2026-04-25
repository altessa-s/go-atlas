// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package pprof

import (
	"github.com/altessa-s/go-atlas/transport/http/server/router"

	stdpprof "net/http/pprof"
)

// Mount registers Go pprof debug handlers (index, cmdline, profile,
// symbol, trace, and all standard profiles) on r under `/pprof`.
// Returns the mounted subrouter for further chaining.
//
// pprof exposes runtime internals (heap, goroutine, CPU profiles) that
// can leak data and enable cheap denial-of-service via expensive
// collection. Mount this only on an internal admin port or behind
// authentication.
func Mount(r router.Router) router.Router {
	debugRouter := r.PathPrefix("/pprof").Subrouter()

	debugRouter.HandleFunc("/", stdpprof.Index)
	debugRouter.HandleFunc("/cmdline", stdpprof.Cmdline)
	debugRouter.HandleFunc("/profile", stdpprof.Profile)
	debugRouter.HandleFunc("/symbol", stdpprof.Symbol)
	debugRouter.HandleFunc("/trace", stdpprof.Trace)
	debugRouter.Handle("/allocs", stdpprof.Handler("allocs"))
	debugRouter.Handle("/block", stdpprof.Handler("block"))
	debugRouter.Handle("/goroutine", stdpprof.Handler("goroutine"))
	debugRouter.Handle("/heap", stdpprof.Handler("heap"))
	debugRouter.Handle("/mutex", stdpprof.Handler("mutex"))
	debugRouter.Handle("/threadcreate", stdpprof.Handler("threadcreate"))

	return debugRouter
}
