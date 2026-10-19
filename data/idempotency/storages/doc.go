// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package storages defines the Storage interface for idempotency key persistence.
// Implementations are in subpackages: memory, mongo, nats, redis; the
// storagetest subpackage is their conformance kit.
//
// Example:
//
//	var store storages.Storage = redis.New(client)
//	acquired, existing, token, err := store.AttemptLock(ctx, "request-123", inProgress)
//	// ... process the request when acquired, then:
//	err = store.Complete(ctx, "request-123", result, token)
package storages
