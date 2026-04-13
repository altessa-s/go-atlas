// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package dispatch

import "context"

// Sink is the destination for batched items processed by Engine. Sinks
// receive at-least-once delivery: a single item may appear in StoreBatch
// more than once after a crash-and-replay sequence. Implementations should
// be idempotent on the records' natural identity (e.g. event ID).
type Sink[T any] interface {
	// StoreBatch persists a batch of items. The batch is non-empty.
	StoreBatch(ctx context.Context, items []T) error
}

// Codec serializes items for WAL durability and decodes them on recovery.
// Implementations should be deterministic and stable across process
// restarts; encode and decode must round-trip exactly.
type Codec[T any] interface {
	Encode(item T) ([]byte, error)
	Decode(b []byte) (T, error)
}

// DropHandler is invoked when an item is dropped at submit time due to a
// full in-memory buffer (and back-pressure mode is disabled).
type DropHandler[T any] func(item T)
