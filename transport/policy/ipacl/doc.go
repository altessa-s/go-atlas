// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package ipacl provides IP-based access control logic shared by the gRPC
// interceptor and HTTP middleware layers.
//
// A [Registry] holds per-endpoint and pattern-based [AccessRule] entries.
// Each rule carries an allowlist and a denylist of CIDR prefixes. The
// registry-level [Policy] determines the default action when an IP matches
// neither list.
//
// The evaluation order within a matched rule is:
//  1. IP in Denylist  → deny  (deny always wins)
//  2. IP in Allowlist → allow
//  3. Neither         → fall back to [Registry] policy
//
// Registry must be fully configured before concurrent use. After
// initialization, [Registry.Evaluate] and [Registry.Lookup] are safe for
// concurrent reads from multiple goroutines, but mutation methods
// ([Registry.Register], [Registry.RegisterPattern], [Registry.RegisterEndpoints],
// [Registry.SetDefault]) must not be called concurrently with reads.
package ipacl
