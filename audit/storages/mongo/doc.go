// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package mongo provides a MongoDB-backed [audit.Storage] implementation.
// Events are persisted in a configurable collection with automatic index
// creation for efficient queries by time range, actor, resource, request ID
// and trace ID. [New] creates the indexes missing from an existing
// collection too.
//
// Store and StoreBatch reject nil events before writing and treat replayed
// events idempotently by Event.ID (_id) only: duplicate-key (11000) errors are
// verified against the primary with majority read concern, and direct callers
// must supply unique IDs. Unverified unique conflicts wrap
// [github.com/altessa-s/go-atlas-core/errors.ErrConflict], write-concern and
// transient network failures wrap
// [github.com/altessa-s/go-atlas-core/errors.ErrUnavailable], and raw server
// error messages are never exposed.
package mongo
