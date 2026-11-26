// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package memory provides a bounded in-memory [audit.Storage] backed by a ring
// buffer.
//
// It is intended for testing, local development, and safe fallback when no
// persistent backend is configured; all events live in process memory and are
// lost on exit.
//
// # Capacity and Eviction
//
// By default [New] (and the zero-value [Storage]) retains up to
// [DefaultMaxEvents] (10,000) events. When the buffer is full, [Storage.Store]
// and [Storage.StoreBatch] evict the oldest stored events first. Configure the
// capacity with [WithMaxEvents]. StoreBatch rejects any batch containing a nil
// event before writing or evicting.
//
//	store := memory.New(memory.WithMaxEvents(5000))
package memory
