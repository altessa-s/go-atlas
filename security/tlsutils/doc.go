// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package tlsutils provides TLS certificate loading and secure configuration utilities.
// Supports multiple certificate sources (file, Vault, Let's Encrypt), private key
// formats (PKCS#8, PKCS#1, SEC1), and enforces modern TLS 1.2+ defaults.
//
// Example:
//
//	cert, _ := tlsutils.LoadFromFile("server.key", "server.crt", "password")
//	config := tlsutils.DefaultTLSConfig()
//	config.Certificates = []tls.Certificate{*cert}
//	caPool, _ := tlsutils.BuildCAPool(true, "custom-ca.crt")
//	config.RootCAs = caPool
package tlsutils
