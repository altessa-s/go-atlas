// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package idempotency holds the shared kernel of the HTTP middleware and
// gRPC interceptor idempotency twins: the default key-format validator,
// its sentinel error, the canonical header/metadata names, and the
// storage-key format.
//
// # Storage-key ABI
//
// [BuildStorageKey] composes the tuple ("transport", service, key) with
// data/idempotency.ComposeKey into the length-prefixed v2 layout, e.g.
// "idk:v2:9:transport17:users.UserService36:{key}", so distinct
// (service, key) pairs never share a storage key. Keys built with it persist
// in external backends (Redis, NATS KV, memory), so the format is an
// external ABI. The v2 layout is frozen; an incompatible format needs a new
// version and a migration. Both transport packages pin it with
// literal-string tests.
//
// # Usage
//
//	if err := idempotency.DefaultKeyValidator(key); err != nil {
//		return err // errors.Is(err, idempotency.ErrInvalidFormat)
//	}
//	storageKey := idempotency.BuildStorageKey("users.UserService", key)
package idempotency
