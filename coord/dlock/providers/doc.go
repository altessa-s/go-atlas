// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package providers defines the Provider and Lock interfaces for distributed locking.
// Implementations are in subpackages: nats, noop.
//
// Example:
//
//	var p providers.Provider = nats.New(js)
//	lock, _ := p.Lock(ctx, "my-resource")
//	defer lock.Release(ctx)
package providers
