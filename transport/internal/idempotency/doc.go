// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package idempotency holds the shared kernel of the HTTP middleware and
// gRPC interceptor idempotency twins: the key-format validators
// ([DefaultKeyValidator], [OpaqueKeyValidator], [PatternKeyValidator],
// bounded by [DefaultMaxKeyLength]), their sentinel error, the canonical
// header/metadata names, the storage-key format, the per-request [Policy]
// (with [Policy.Mandatory]), the lock-outcome classification and the record
// stored for response replay.
//
// # Lock outcomes
//
// [ClassifyLock] turns an AttemptLock result into a [LockOutcome] and, for
// [LockCompleted], the [Record] decoded from the stored data. Each
// transport answers every outcome its own way: status codes, fail-open
// policy, headers and replay.
//
// # Response replay
//
// With replay on, a transport stores a [Record] built by [NewRecord] with
// the completed key; without it, [EntityIDData], the bare entity ID written
// before replay existed. [DecodeRecord] reads either back, and
// [Record.HasGRPCResponse] / [Record.HasHTTPResponse] report whether the
// record holds a response for the request's binding. gRPC storage keys are
// scoped by service only and HTTP keys by METHOD:path, so a response is
// replayed only to a request with the same binding. A response larger than
// the transport's configured replay size limit (default
// [DefaultMaxReplaySize]) is not stored.
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
