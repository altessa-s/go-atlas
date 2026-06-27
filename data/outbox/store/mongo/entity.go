// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxstore

import "time"

// event is the internal MongoDB document representation of an outbox.Event.
//
// Timestamps are stored as BSON Date (not Unix integers) so that clock-skew-safe
// comparisons against the MongoDB server clock ($$NOW) are well-typed. Optional
// timestamps are pointers: a nil pointer is omitted (absent field), which the
// queries treat distinctly from a set date.
type event struct {
	Id            string            `bson:"_id"`                       // Primary key.
	Status        string            `bson:"status"`                    // Processing status.
	Event         []byte            `bson:"event"`                     // Event payload (BSON field name unchanged for DB compat).
	CreatedAt     time.Time         `bson:"created_at"`                // Creation timestamp (BSON Date).
	PublishedAt   *time.Time        `bson:"published_at,omitempty"`    // Publication timestamp; nil until processed.
	LastError     *string           `bson:"error,omitempty"`           // Last error message.
	Attempts      uint32            `bson:"attempts"`                  // Dispatch attempt count.
	LastAttemptOn *time.Time        `bson:"last_attempt_on,omitempty"` // Last attempt timestamp; nil until first attempt.
	Topic         string            `bson:"topic"`                     // Maps to Event.Key (BSON name unchanged for DB compat).
	Metadata      map[string]string `bson:"metadata,omitempty"`        // Kept for DB compat; not mapped to outbox.Event.
	LockedOn      *time.Time        `bson:"locked_on,omitempty"`       // Lock timestamp; nil when unlocked.
	ExpiresAt     *time.Time        `bson:"expires_at,omitempty"`      // Expiration timestamp; nil means no expiration.
}
