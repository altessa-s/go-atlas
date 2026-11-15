// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the vault section of the secrets configuration to a
// Vault KV provider for factory/ext/kryptos.
//
// The secrets factory builds only the in-memory provider; register this one
// for the vault provider type:
//
//	manager, err := secretsfactory.New(&cfg.Secrets).
//		UseProvider(secretsconfig.ProviderVault, vaultfactory.Builder(vaultClient)).
//		Build(ctx)
package factory
