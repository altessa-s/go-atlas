// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package tlsvault provides HashiCorp Vault-based TLS certificate provider.
// Integrates with Vault PKI engine for dynamic certificate generation and automatic renewal.
//
// Example:
//
//	provider, _ := tlsvault.New(
//		tlsvault.WithEndpoint("https://vault.example.com"),
//		tlsvault.WithStaticToken("token"),
//		tlsvault.WithRole("web-server"),
//	)
//	defer provider.Close()
//	tlsConfig, _ := provider.TLSConfig()
package tlsvault
