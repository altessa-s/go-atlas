// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of TLS configurations and providers.
// It integrates with config.TlsClient and config.TlsProvider to create TLS configurations
// and certificate providers with their respective backends (File, Vault, Let's Encrypt).
//
// Example:
//
//	f := factory.New()
//	providers, err := f.CreateProvidersFromConfig(cfg.TlsProvider, vaultClient, "./certs")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer providers.Close(ctx, nil)
package factory
