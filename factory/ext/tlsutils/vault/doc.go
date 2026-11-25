// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory maps the vault section of the TLS provider configuration to
// a Vault PKI provider for factory/ext/tlsutils.
//
// The TLS factory builds only the file and Let's Encrypt providers; register
// this one for the vault provider type:
//
//	providers, err := tlsfactory.New(cfg.TLSProvider).
//		UseProvider(tlsproviders.ProviderTypeVault, vaultfactory.Builder(vaultClient)).
//		Build(ctx)
package factory
