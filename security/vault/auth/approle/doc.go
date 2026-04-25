// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package approle provides AppRole authentication for HashiCorp Vault.
// Designed for automated workflows where applications authenticate using
// a role ID and secret ID pair.
//
// Example:
//
//	authMethod := approle.New("role-id", "secret-id")
//	// With custom mount path:
//	authMethod := approle.New("role-id", "secret-id",
//	    approle.WithMountPath("custom-approle"))
package approle
