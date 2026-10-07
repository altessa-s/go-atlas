// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package endpointrule provides the endpoint-to-rule resolution shared by the
// access-control registries in transport/internal (ipacl, geoacl).
//
// A [Registry] maps endpoints to rules of any type through exact names,
// regular-expression patterns and an optional default. [Registry.Lookup]
// resolves an endpoint in that order: an exact match wins, then the first
// matching pattern in registration order, then the default rule.
//
// # Concurrency
//
// A Registry must be fully configured before concurrent use. Lookup is then
// safe for concurrent reads, but the registration methods must not run
// concurrently with reads.
//
//	var r endpointrule.Registry[Rule]
//	r.Register("/svc.Method", &Rule{})
//	r.RegisterPattern(regexp.MustCompile(`^/admin\.`), &Rule{})
//	rule, ok := r.Lookup("/admin.Users")
package endpointrule
