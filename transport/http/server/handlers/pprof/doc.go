// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package pprof mounts Go's runtime/pprof debug handlers on a
// [router.Router]. It is opt-in by design; read the Security section
// before mounting it.
//
// # Security
//
// [Mount] performs NO authentication or authorization. The endpoints it
// registers MUST be placed behind an auth middleware or bound to a
// non-public listener (localhost or an internal admin port). Exposed
// publicly, they hand any caller heap and goroutine dumps and CPU
// profiles — which can contain secrets held in process memory — and
// enable cheap denial of service by triggering expensive profile
// collection.
//
// # Usage
//
//	import pprofh "github.com/altessa-s/go-atlas/transport/http/server/handlers/pprof"
//
//	pprofh.Mount(adminRouter) // exposes /pprof/* on adminRouter
package pprof
