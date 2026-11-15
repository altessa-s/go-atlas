// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package geoacl provides geographic access control logic shared by the gRPC
// interceptor and HTTP middleware layers.
//
// A [Registry] holds per-endpoint and pattern-based [AccessRule] entries.
// Each rule carries allow and deny lists for continents, countries, and
// regions. The registry-level [Policy] determines the default action when
// a location matches neither list.
//
// The evaluation order within a matched rule is (most specific to least):
//  1. Region in DenyRegions     → deny
//  2. Country in DenyCountries  → deny
//  3. Continent in DenyContinents → deny
//  4. Region in AllowRegions    → allow
//  5. Country in AllowCountries → allow
//  6. Continent in AllowContinents → allow
//  7. Neither                   → fall back to [Registry] policy
//
// Geographic information is resolved via the [GeoResolver] interface.
// If the resolver returns an error, it is propagated to the caller,
// allowing the transport layer to apply its fallback behavior.
//
// Registry must be fully configured before concurrent use. After
// initialization, [Registry.Evaluate] and [Registry.Lookup] are safe for
// concurrent reads from multiple goroutines, but mutation methods
// ([Registry.Register], [Registry.RegisterPattern], [Registry.RegisterEndpoints],
// [Registry.SetDefault]) must not be called concurrently with reads.
package geoacl
