// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package tlsle provides Let's Encrypt TLS certificate provider using HTTP-01 ACME challenges.
// Wraps autocert for automatic certificate management with renewal and caching support.
//
// Example:
//
//	provider, _ := tlsle.New(
//		tlsle.WithDomains("example.com"),
//		tlsle.WithEmail("admin@example.com"),
//	)
//	defer provider.Close()
//	go provider.StartHTTPServer(":80", nil)
//	tlsConfig, _ := provider.TLSConfig()
package tlsle
