// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package msg

import (
	"time"

	"github.com/altessa-s/go-atlas/core/collections/slices"
	"github.com/altessa-s/go-atlas/core/types/nilcheck"
)

// NoAckTimeout indicates no timeout for message acknowledgment.
const NoAckTimeout = time.Duration(-1)

// Message represents a single message in the broker system.
// It contains metadata, payload data, topic, TTL, and acknowledgment settings.
type Message struct {
	// Metadata associated with the message.
	Metadata Meta
	// Data is the main content/payload of the message.
	Data []byte
	// Topic under which the message is categorized or sent.
	Topic string
	// TTL (Time-To-Live) for the message, indicating how long it should be retained
	// before being discarded. A zero or negative value might indicate no TTL.
	TTL time.Duration
	// AckTimeout is the duration within which a message acknowledgment (Ack/Nak) is expected.
	// NoAckTimeout can be used to specify no timeout.
	AckTimeout time.Duration

	acker Acker // The acknowledgment mechanism for the message
}

// NewMessage creates a new message with the provided topic and data.
// Default metadata including creation time is added automatically.
//
// Example:
//
//	m := msg.NewMessage("events", []byte(`{"type":"created"}`))
func NewMessage(topic string, data []byte, opt ...Option) *Message {
	return NewMessageWithMeta(topic, data, nil, opt...)
}

// NewMessageWithMeta creates a new message with topic, data, and initial metadata.
// Metadata is combined with defaults (creation time) and deduplicated. Data is copied.
//
// Example:
//
//	meta := msg.Meta{{Key: "trace_id", Value: "abc123"}}
//	m := msg.NewMessageWithMeta("events", data, meta)
func NewMessageWithMeta(topic string, data []byte, metadata Meta, opt ...Option) *Message {
	// Ensure created time exists, but allow callers to override it by providing MetaKeyMessageCreatedTime.
	meta := make([]MetaData, 0, len(metadata)+1)
	meta = append(meta, metadata...)
	if !hasMetaKeyWithNonEmptyValue(meta, MetaKeyMessageCreatedTime) {
		meta = append(meta, MetaData{Key: MetaKeyMessageCreatedTime, Value: time.Now().UTC().Format(MessageCreatedTimeFormat)})
	}

	m := &Message{
		Topic:      topic,
		AckTimeout: NoAckTimeout,
		Data:       make([]byte, len(data)), // Message owns its data
		Metadata:   slices.DeduplicateBy(meta, func(v MetaData) string { return v.Key }),
	}

	copy(m.Data, data) // Copy data content

	for _, o := range opt {
		o(m)
	}

	return m
}

func hasMetaKeyWithNonEmptyValue(meta Meta, key string) bool {
	for i := range meta {
		if meta[i].Key == key && meta[i].Value != "" {
			return true
		}
	}
	return false
}

// ackerIsNil checks if the acker is nil (interface or underlying value).
func (m *Message) ackerIsNil() bool {
	return nilcheck.IsNil(m.acker)
}

// Ack sends a positive acknowledgment indicating successful processing.
// Returns nil without action if no acker is configured.
//
// Example:
//
//	err := m.Ack()
func (m *Message) Ack() error {
	if m.ackerIsNil() {
		return nil
	}
	return m.acker.Ack()
}

// Nak sends a negative acknowledgment indicating processing failure.
// Optional duration specifies delay before redelivery. Returns nil if no acker.
//
// Example:
//
//	err := m.Nak(5 * time.Second) // retry after 5 seconds
func (m *Message) Nak(duration ...time.Duration) error {
	if m.ackerIsNil() {
		return nil
	}
	return m.acker.Nak(duration...)
}

// NakWithBackOff sends a negative acknowledgment with a delay computed by the provided
// BackOffFunc based on the current delivery attempt count. Returns nil if no acker.
//
// Example:
//
//	err := m.NakWithBackOff(func(attempt uint64) time.Duration {
//		delays := []time.Duration{5 * time.Second, 30 * time.Second, 5 * time.Minute}
//		return delays[min(int(attempt)-1, len(delays)-1)]
//	})
func (m *Message) NakWithBackOff(backOff BackOffFunc) error {
	if m.ackerIsNil() {
		return nil
	}
	return m.acker.NakWithBackOff(backOff)
}

// Term sends a terminal acknowledgment indicating the message should not be redelivered.
// Optional reason can be provided. Returns nil if no acker is configured.
//
// Example:
//
//	err := m.Term("invalid payload")
func (m *Message) Term(reason ...string) error {
	if m.ackerIsNil() {
		return nil
	}
	return m.acker.Term(reason...)
}

// InProgress sends a heartbeat indicating processing is ongoing.
// Prevents premature redelivery for long-running tasks. Returns nil if no acker.
//
// Example:
//
//	err := m.InProgress()
func (m *Message) InProgress() error {
	if m.ackerIsNil() {
		return nil
	}
	return m.acker.InProgress()
}

// Acker defines the interface for message acknowledgment mechanisms.
// It provides methods for positive, negative, and terminal acknowledgments.
// Implementations must be safe for concurrent use; [Message.Ack],
// [Message.Nak], and [Message.Term] delegate directly to these methods.
type Acker interface {
	// Ack sends a positive acknowledgment for successful processing.
	Ack() error

	// Nak sends a negative acknowledgment with optional redelivery delay.
	Nak(delay ...time.Duration) error

	// NakWithBackOff sends a negative acknowledgment with a delay computed by backOff
	// based on the current delivery attempt count.
	NakWithBackOff(backOff BackOffFunc) error

	// Term sends a terminal acknowledgment indicating no redelivery.
	Term(reason ...string) error

	// InProgress sends a heartbeat for long-running tasks.
	InProgress() error
}
