// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package tlsfile provides file-based TLS certificate provider with optional
// automatic reloading. Supports password-protected keys and OCSP stapling.
//
// Example:
//
//	provider, _ := tlsfile.NewWithCertAndKey("cert.pem", "key.pem", "",
//	    tlsfile.WithWatcherEnabled(true),
//	    tlsfile.WithLogger(slog.Default()),
//	)
//	defer provider.Close()
//	config, _ := provider.TLSConfig()
package tlsfile
