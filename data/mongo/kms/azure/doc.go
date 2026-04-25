// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package kmsazure provides Azure Key Vault integration for MongoDB CSFLE.
// Credentials are stored securely and automatically cleared when no longer needed.
//
// Example:
//
//	p := kmsazure.New(tenantId, clientId, clientSecret, keyVaultEndpoint, keyName)
//	defer p.Clear()
package kmsazure
