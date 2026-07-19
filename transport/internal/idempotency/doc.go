// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package idempotency holds the shared kernel of the HTTP middleware and
// gRPC interceptor idempotency twins: the default key-format validator,
// its sentinel error, the canonical header/metadata names, and the
// storage-key format.
//
// # Storage-key ABI
//
// [BuildStorageKey] owns the "idk:{service}:{key}" layout. Keys built with
// it persist in external backends (Redis, NATS KV, memory), so the format
// is an external ABI and must remain byte-for-byte stable. Both transport
// packages pin it with literal-string tests.
//
// # Usage
//
//	if err := idempotency.DefaultKeyValidator(key); err != nil {
//		return err // errors.Is(err, idempotency.ErrInvalidFormat)
//	}
//	storageKey := idempotency.BuildStorageKey("users.UserService", key)
package idempotency
