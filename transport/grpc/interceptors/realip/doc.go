// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package realip provides gRPC interceptors for extracting real client IP from proxy headers.
// Processes X-Forwarded-For, X-Real-IP, CF-Connecting-IP headers from gRPC metadata.
//
// Example:
//
//	extractor := clientip.NewExtractor(clientip.WithTrustedProxies(netip.MustParsePrefix("10.0.0.0/8")))
//	server := grpc.NewServer(grpc.UnaryInterceptor(realip.ServerUnaryInterceptor(extractor)))
//	// In handler: clientIP := clientip.FromContext(ctx)
package realip
