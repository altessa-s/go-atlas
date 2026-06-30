// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package factory builds a SPIFFE Workload API source from configuration.
//
// It derives the peer authorizer and core options from a [config.SPIFFE] and
// constructs a connected provider, mirroring the factory subpackages across the
// auth stack. The authorizer accepts a peer that belongs to any configured
// trust domain or matches any configured ID; with neither configured it fails
// closed.
//
// # Usage
//
//	provider, err := factory.New(&cfg.SPIFFE).Provider(ctx)
//	if err != nil {
//		return err
//	}
//	defer provider.Close()
//	server := &http.Server{TLSConfig: provider.MTLSServerConfig()}
package factory
