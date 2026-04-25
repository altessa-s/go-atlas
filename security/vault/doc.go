// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package vault provides high-level HashiCorp Vault client with pluggable authentication and automatic token renewal.
// Wraps official Vault API client with support for AppRole, Token, and UserPass authentication methods.
//
// Example:
//
//	client, _ := vault.New(
//		vault.WithAuthMethod(approle.New("role-id", "secret-id")),
//		vault.WithLogger(slog.Default()),
//	)
//	client.RunRenewal()
//	defer client.StopRenewal()
package vault
