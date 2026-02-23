// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory provides configuration-based creation of OIDC providers.
// It integrates with config.OIDC to create Provider instances with validation
// options, presets, caching, and introspection support.
//
// Example:
//
//	f := factory.New(
//		factory.WithScheduler(scheduler),
//		factory.WithLogger(logger),
//		factory.WithTokenCache(tokenCache),
//	)
//	provider, err := f.CreateProviderFromConfig(ctx, cfg, revocationStorage)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer provider.Close()
package factory
