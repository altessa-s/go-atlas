// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package kvstore provides a common interface and adapter for key-value based
// cursor storage implementations. It eliminates code duplication between
// different backends (Redis, NATS, etc.) by abstracting the JSON serialization
// and error handling logic.
//
// Example:
//
//	backend := natsbackend.New(kv)
//	storage := kvstore.NewJSONStorage(backend, "nats")
package kvstore
