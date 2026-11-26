// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package metadata provides gRPC call metadata extraction and context injection.
// It serves as a foundation for other interceptors by providing a unified view
// of the RPC call, including service name, method name, stream type, and timing.
//
// [CallMetadata] is created once per RPC by [NewCallMetadata] and stored in the
// context via [NewContext]. Subsequent interceptors retrieve it with [FromContext]
// or use the convenience helper [EnsureInContext] which creates metadata on
// demand. Registered server method names are cached process-wide in a bounded
// [sync.Map] for efficient repeated lookups; unknown and client method names
// are parsed without interning.
//
// The [Interceptor] function returns a [driver.DrivenInterceptor] that the
// [interceptors.Chain] automatically prepends, so all downstream
// interceptors have access to [CallMetadata] without manual setup.
//
// Example:
//
//	ctx, meta := metadata.EnsureInContext(ctx, fullMethod, info)
//	fmt.Println(meta.ServiceName, meta.MethodName, meta.Duration())
package metadata
