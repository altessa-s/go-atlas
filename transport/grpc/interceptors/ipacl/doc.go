// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package ipacl provides gRPC interceptors for IP-based access control.
// It enforces allowlist and denylist rules per endpoint, blocking or
// permitting requests based on the client IP resolved by the "realip"
// interceptor.
//
// Use [ServerInterceptor] (or the standalone [ServerUnaryInterceptor] /
// [ServerStreamInterceptor]) to apply IP access control. The interceptor
// declares a dependency on the "realip" interceptor so that the client IP
// is always available in the context.
//
// When a request is denied, the interceptor responds with
// [codes.PermissionDenied]. When no valid client IP is available, the
// configured fallback behavior (default: deny) determines the outcome.
//
// Example:
//
//	registry := ipacl.NewRegistry(ipacl.PolicyDeny)
//	registry.Register("/admin.AdminService/Delete", &ipacl.AccessRule{
//	    Allowlist: []netip.Prefix{netip.MustParsePrefix("10.1.1.0/24")},
//	})
//
//	interceptor := ipacl.ServerInterceptor(registry,
//	    ipacl.WithFallbackBehavior(fallback.Deny),
//	)
package ipacl
