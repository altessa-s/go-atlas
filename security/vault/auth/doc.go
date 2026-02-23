// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package auth provides pluggable authentication methods for HashiCorp Vault
// with automatic token renewal and lifecycle management.
//
// Example:
//
//	authMethod := approle.New("role-id", "secret-id")
//	authenticator := auth.NewAuthenticator(vaultClient, authMethod,
//	    auth.WithBackoffBase(2*time.Second),
//	)
//	go authenticator.Run(ctx)
//	<-authenticator.FirstRenewCh()
package auth
