// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package token provides direct token-based authentication for HashiCorp Vault.
// Validates and uses existing Vault tokens without credential exchange.
//
// Example:
//
//	authMethod := token.New("hvs.CAESIJ...")
//	secret, _ := authMethod.Authenticate(ctx, vaultClient)
//	if secret.Auth.Renewable {
//	    log.Printf("Token TTL: %d seconds", secret.Auth.LeaseDuration)
//	}
package token
