// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package userpass provides username/password authentication for HashiCorp Vault.
// Suitable for interactive applications and development environments.
//
// Example:
//
//	authMethod := userpass.New("username", "password")
//	// With custom mount path:
//	authMethod := userpass.New("username", "password",
//	    userpass.WithMountPath("custom-userpass"))
package userpass
