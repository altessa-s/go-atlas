// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package handler

import (
	"net/http/pprof"

	"github.com/altessa-s/go-atlas/transport/http/server/router"
)

// Pprof registers pprof debug handlers (index, cmdline, profile, symbol,
// trace, and all standard profiles) on the provided router.
// Returns a subrouter mounted at /pprof. This should typically be exposed
// only on an internal admin port.
func Pprof(r router.Router) router.Router {
	debugRouter := r.PathPrefix("/pprof").Subrouter()

	debugRouter.HandleFunc("/", pprof.Index)
	debugRouter.HandleFunc("/cmdline", pprof.Cmdline)
	debugRouter.HandleFunc("/profile", pprof.Profile)
	debugRouter.HandleFunc("/symbol", pprof.Symbol)
	debugRouter.HandleFunc("/trace", pprof.Trace)
	debugRouter.Handle("/allocs", pprof.Handler("allocs"))
	debugRouter.Handle("/block", pprof.Handler("block"))
	debugRouter.Handle("/goroutine", pprof.Handler("goroutine"))
	debugRouter.Handle("/heap", pprof.Handler("heap"))
	debugRouter.Handle("/mutex", pprof.Handler("mutex"))
	debugRouter.Handle("/threadcreate", pprof.Handler("threadcreate"))

	return debugRouter
}
