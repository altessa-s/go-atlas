// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package msg

import (
	"time"
)

// Option is a functional option for configuring a Message during creation.
type Option func(*Message)

// WithAcker sets the acknowledgment mechanism for the message.
func WithAcker(a Acker) Option {
	return func(m *Message) {
		m.acker = a
	}
}

// WithAckTimeout sets the acknowledgment timeout for the message.
// Use NoAckTimeout to indicate no timeout.
func WithAckTimeout(timeout time.Duration) Option {
	return func(m *Message) {
		m.AckTimeout = timeout
	}
}

// WithTTL sets the Time-To-Live for the message.
// Zero or negative TTL may be interpreted as no expiration.
func WithTTL(ttl time.Duration) Option {
	return func(m *Message) {
		m.TTL = ttl
	}
}
