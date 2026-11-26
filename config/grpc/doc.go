// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package grpcconfig defines the gRPC server schema and the schemas of its
// interceptors.
//
// Schemas carry yaml and default tags read by
// github.com/altessa-s/go-nomos/loader, expose Default* constructors
// where defaults exist, and implement Validate; component factories map them
// to generated options.
//
// Key types: [TLS], [KeepAliveEnforcementPolicy], [KeepAlive], [Config],
// [AuthInterceptor], [CacheInterceptor], [InterceptorFilter]. The cache schema
// lists the exact RPCs eligible for shared caching without authentication.
package grpcconfig
