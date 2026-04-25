// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package cache provides gRPC server interceptors for response caching with
// compression support.
//
// Use [ServerInterceptor] to create a caching interceptor backed by any
// [Cacher] implementation (redis, freecache, lru, noop). Configure per-method
// caching with [MethodConfig], set caching policies with [DecisionFunc] (see
// [DefaultSuccessOnlyDecision] and [DefaultSuccessAndErrorDecision]), and
// generate cache keys with [KeyGenerator] (see [DefaultKeyGenerator] and
// [NewKeyGenerator]).
//
// The interceptor uses the driven interceptor pattern and supports unary RPCs
// only; streaming methods bypass the cache. When cache headers are enabled
// (see WithCacheHeaders), the interceptor sets [HeaderKey] / [HeaderHit] /
// [HeaderMiss] in gRPC response metadata.
//
// Serialization is handled by [Serializer] (default: [DefaultSerializer]) with
// optional gzip compression via the [compression] sub-package. The
// [MetadataProcessor] hook allows custom cache key partitioning from gRPC
// metadata (e.g., extracting tenant IDs from JWT tokens).
//
// Example:
//
//	interceptor := cache.ServerInterceptor(memCache,
//	    cache.WithMethodConfig(
//	        cache.Method{
//	            Method:        "/api.UserService/GetUser",
//	            ResponseProto: &pb.User{},
//	            TTL:           5 * time.Minute,
//	        },
//	    ),
//	    cache.WithCompressionPreset(cache.CompressionPresetBalanced),
//	)
//
//	opts, _ := interceptors.NewChain(interceptor).ServerOptions()
//	server := grpc.NewServer(opts...)
package cache
