// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of gRPC servers and interceptors.
//
// Use [New] to create a [Factory], then call Create* methods to build
// server components from [config] structs. All created components inherit
// the factory's logger. When TLS is required, the factory resolves
// certificates through configured [tlsproviders.Providers].
//
// # Server Creation
//
//   - [Factory.CreateServerFromConfig] -- builds a [grpcserver.Server] from
//     a [config.Grpc] struct
//
// # Interceptor Creation (from configuration)
//
// Each method returns (nil, nil) when the configuration is nil or disabled,
// allowing callers to pass the result directly to
// [interceptors.ServerConditionalInterceptor].
//
//   - [Factory.CreateLoggerInterceptorFromConfig]
//   - [Factory.CreatePrometheusInterceptorFromConfig]
//   - [Factory.CreateTracingInterceptorFromConfig]
//   - [Factory.CreateRealIPInterceptorFromConfig]
//   - [Factory.CreateRecoveryInterceptorFromConfig]
//   - [Factory.CreateRequestIDInterceptorFromConfig]
//   - [Factory.CreateLimiterInterceptorFromInterConfig]
//   - [Factory.CreateIdempotencyInterceptorFromInterConfig]
//   - [Factory.CreateCacheInterceptorFromConfig]
//   - [Factory.CreateAuthInterceptorFromConfig]
//   - [Factory.CreateHealthInterceptorFromConfig]
package factory
