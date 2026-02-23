// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of Vault clients.
// It integrates with the config.Vault configuration to create
// Vault instances with their respective authentication methods.
//
// Example:
//
//	f := factory.New(
//	    factory.WithLogger(logger),
//	    factory.WithTlsConfig(tlsConfig),
//	)
//	v, err := f.CreateVaultFromConfig(ctx, cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer v.StopRenewal()
package factory
