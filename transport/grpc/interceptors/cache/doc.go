// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package cache provides gRPC server interceptors for response caching with
// compression support.
//
// Use [ServerInterceptor] to create a caching interceptor backed by any
// [github.com/altessa-s/go-atlas/cache.Cacher] implementation (redis, freecache, lru, noop). Configure per-method
// caching with [MethodPolicy], set caching policies with [DecisionFunc] (see
// [DefaultSuccessOnlyDecision] and [DefaultSuccessAndErrorDecision]), and
// generate cache keys with [KeyGenerator] (see [DefaultKeyGenerator] and
// [NewKeyGenerator]).
//
// The interceptor uses the driven interceptor pattern and supports unary RPCs
// only; streaming methods bypass the cache. When cache headers are enabled
// (see WithCacheHeaders), the interceptor sets [HeaderKey] / [HeaderHit] /
// [HeaderMiss] in gRPC response metadata.
//
// A dependency-ordered chain runs authentication and access controls before
// this interceptor. The default identity uses the current principal's tenant,
// subject, sorted scopes and roles, and the authentication method and verified
// issuer when available. Use [WithIdentityFunc] for custom credentials or
// other verified attributes that affect the response. Authenticated calls
// without an identifiable principal and auth-ignored calls under
// [WithAuthRequired] bypass caching. On a server without auth, only methods
// explicitly listed with [WithPublicMethods] can share cached responses.
//
// Cache keys and their independent verifiers cover the method, identity,
// serialized request, and response discriminators. The default metadata
// discriminators are API version, feature flags, and the read-mask header.
// [WithReadMaskHeader] adds a custom header. [Serializer] handles
// responses with optional gzip compression, and cached gRPC errors preserve
// their status details.
//
// Example:
//
//	interceptor, err := cache.ServerInterceptor(memCache,
//	    cache.WithMethodPolicy(
//	        cache.MethodPolicy{
//	            Name:          "/api.Catalog/GetProduct",
//	            ResponseProto: &pb.Product{},
//	        },
//	    ),
//	    cache.WithPublicMethods("/api.Catalog/GetProduct"),
//	    cache.WithCompressionPreset(compression.PresetBalanced),
//	)
//
//	if err != nil {
//	    return err
//	}
//	opts, _ := interceptors.NewChain(interceptor).ServerOptions()
//	server := grpc.NewServer(opts...)
package cache
