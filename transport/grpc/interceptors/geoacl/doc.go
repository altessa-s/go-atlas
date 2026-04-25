// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package geoacl provides gRPC interceptors for geographic access control.
// It enforces allow and deny rules per endpoint based on the geographic
// location of the client IP, resolved via a [geoacl.GeoResolver].
//
// Use [ServerInterceptor] (or the standalone [ServerUnaryInterceptor] /
// [ServerStreamInterceptor]) to apply geographic access control. The interceptor
// declares a dependency on the "realip" interceptor so that the client IP
// is always available in the context.
//
// When a request is denied, the interceptor responds with
// [codes.PermissionDenied]. When no valid client IP is available or the
// geo resolver returns an error, the configured fallback behavior
// (default: deny) determines the outcome.
//
// Example:
//
//	registry := geoacl.NewRegistry(geoacl.PolicyDeny)
//	registry.Register("/admin.AdminService/Delete", &geoacl.AccessRule{
//	    AllowCountries: []string{"US", "CA"},
//	})
//
//	interceptor := geoacl.ServerInterceptor(resolver, registry,
//	    geoacl.WithFallbackBehavior(fallback.Deny),
//	)
package geoacl
