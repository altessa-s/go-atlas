// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package providers defines the Provider interface for leader election backends.
// Implementations are in subpackages (e.g., nats).
//
// Example:
//
//	var le providers.Provider = nats.New(js)
//	le.Start(ctx, providers.Config{Key: "my-service", TTL: 10*time.Second})
package providers
