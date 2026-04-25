// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package tlsproviders defines interfaces for TLS certificate providers.
// Implement Provider interface to add support for different certificate sources
// (file, Vault, Let's Encrypt).
//
// Example:
//
//	var provider tlsproviders.Provider
//	config, _ := provider.TLSConfig()
//	defer provider.Close()
package tlsproviders
