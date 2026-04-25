// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package clientip extracts the real client IP address from HTTP and gRPC
// requests by traversing proxy header chains.
//
// When a request arrives through one or more reverse proxies, the direct
// peer address belongs to the last proxy, not the original client.
// [Extractor] walks the configured headers (X-Forwarded-For, CF-Connecting-IP,
// etc.) right-to-left, skipping trusted proxies and private addresses, to
// locate the first public IP that is not part of the trusted infrastructure.
//
// Parsed addresses are cached in an LRU to avoid repeated [netip.ParseAddr]
// calls on hot paths. The cache, trusted-proxy list, and header set are all
// configurable through functional [Option] values.
//
// Use [NewContext] and [FromContext] to propagate the resolved IP through
// request contexts.
//
// Example:
//
//	extractor, _ := clientip.NewExtractor(
//	    clientip.WithTrustedProxies(netip.MustParsePrefix("10.0.0.0/8")),
//	    clientip.WithHeaders("CF-Connecting-IP", "X-Forwarded-For"),
//	)
//	ip := extractor.Extract(ctx, peerIP, headers)
//	ctx = clientip.NewContext(ctx, ip)
package clientip
