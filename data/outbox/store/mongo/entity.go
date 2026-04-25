// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package outboxstore

// event is the internal MongoDB document representation of an outbox.Event.
type event struct {
	Id            string            `bson:"_id"`                  // Primary key.
	Status        string            `bson:"status"`               // Processing status.
	Event         []byte            `bson:"event"`                // Event payload (BSON field name unchanged for DB compat).
	CreatedAt     int64             `bson:"created_at"`           // Creation timestamp (Unix).
	PublishedAt   int64             `bson:"published_at"`         // Publication timestamp (Unix).
	LastError     *string           `bson:"error,omitempty"`      // Last error message.
	Attempts      uint32            `bson:"attempts"`             // Dispatch attempt count.
	LastAttemptOn int64             `bson:"last_attempt_on"`      // Last attempt timestamp (Unix).
	Topic         string            `bson:"topic"`                // Maps to Event.Key (BSON name unchanged for DB compat).
	Metadata      map[string]string `bson:"metadata,omitempty"`   // Kept for DB compat; not mapped to outbox.Event.
	LockedOn      int64             `bson:"locked_on,omitempty"`  // Lock timestamp (Unix).
	ExpiresAt     int64             `bson:"expires_at,omitempty"` // Expiration timestamp (Unix).
}
