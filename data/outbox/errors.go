// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outbox

import "errors"

// Sentinel errors for the outbox package.
var (
	// ErrTaskIDCollision is returned from registerTasks when two or more of
	// the configured scheduler task IDs (dispatch / unlock / expire /
	// cleanup) are equal. The underlying scheduler upserts by ID, so a
	// collision would silently overwrite the first task's Func pointer
	// with the second's — we surface it as a startup error instead.
	ErrTaskIDCollision = errors.New("outbox scheduler task IDs must be distinct")

	// ErrEmptyKey is returned from Save for an event with no routing key.
	// An empty key would be published to an empty subject, which every broker
	// either rejects at publish time or silently routes nowhere.
	ErrEmptyKey = errors.New("outbox event key must not be empty")

	// ErrPayloadTooLarge is returned from Save when Event.Payload exceeds the
	// configured limit — see [WithMaxPayloadBytes]. Rejecting at Save keeps the
	// failure inside the caller's transaction boundary, where it can still be
	// handled, instead of surfacing later as a store or broker error.
	ErrPayloadTooLarge = errors.New("outbox event payload exceeds the configured limit")
)
