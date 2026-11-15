// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package msg

//go:generate go tool optgen generate

import (
	"time"
)

// options configures a Message during creation.
//
// ackTimeout and ttl keep non-positive values: NoAckTimeout is negative and a
// zero or negative TTL means no expiration.
type options struct {
	// acker is the acknowledgment mechanism for the message.
	acker Acker
	// ackTimeout is the acknowledgment timeout; NoAckTimeout means none.
	ackTimeout time.Duration `optgen:"default=NoAckTimeout" optval:"nonpositive"`
	// ttl is the message Time-To-Live; zero or negative means no expiration.
	ttl time.Duration `opt:"TTL" optval:"nonpositive"`
}
