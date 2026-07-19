// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factoryconv provides shared config-to-runtime conversion helpers
// used by the HTTP and gRPC server factories. It translates declarative
// [config] structures (regex pattern lists, IP/CIDR prefix lists, fallback
// behavior strings, IP and geographic ACL rules) into the runtime types
// consumed by transport middlewares and interceptors.
//
// # Helpers
//
//   - [CompilePatterns] compiles string patterns to regexps, skipping
//     invalid ones.
//   - [ParsePrefixes] parses IP/CIDR strings to netip.Prefix values.
//   - [ConvertFallbackBehavior] maps [config.FallbackBehavior] to
//     [fallback.Behavior], failing closed on unknown values.
//   - [BuildIpAclRegistry] / [ConvertIpAclRule] build an [ipacl.Registry]
//     from configuration.
//   - [BuildGeoAclRegistry] / [ConvertGeoAclRule] build a [geoacl.Registry]
//     from configuration.
//
// # Usage
//
//	registry, err := factoryconv.BuildIpAclRegistry(c.DefaultPolicy, c.Rules, c.DefaultRule)
//	if err != nil {
//	    return fmt.Errorf("build IP ACL registry: %w", err)
//	}
package factoryconv
