// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package redistest returns a go-redis client connected to an in-process miniredis for tests; both are closed through tb.Cleanup.
//
// [ServeTopology] supplies ROLE and INFO cluster replies for topology tests.
//
// It lives apart from internal/testhelpers so the heavy server dependencies
// (nats-server, miniredis) stay out of packages that only need generic
// test helpers.
package redistest
