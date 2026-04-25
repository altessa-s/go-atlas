// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package pprof mounts Go's runtime/pprof debug handlers on a
// [router.Router]. It is opt-in by design — pprof leaks runtime
// internals (heap, goroutine, CPU profiles) and enables cheap
// denial-of-service via expensive profile collection. Mount it only on
// an internal admin port or behind authentication.
//
// # Usage
//
//	import pprofh "github.com/altessa-s/go-atlas/transport/http/server/handlers/pprof"
//
//	pprofh.Mount(adminRouter) // exposes /pprof/* on adminRouter
package pprof
