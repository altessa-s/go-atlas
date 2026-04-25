// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package msg

import (
	"iter"
	"time"

	"github.com/altessa-s/go-atlas/core/collections/maps"
)

const (
	// MetaKeyDeduplicateId is the metadata key for deduplication ID.
	MetaKeyDeduplicateId = "deduplicate_id"

	// MetaKeyMessageId is the metadata key for unique message identifier.
	MetaKeyMessageId = "message_id"

	// MetaKeyMessageCreatedTime is the metadata key for message creation timestamp.
	MetaKeyMessageCreatedTime = "message_created_time"
)

// MessageCreatedTimeFormat is the time format (RFC3339) used for MetaKeyMessageCreatedTime.
const MessageCreatedTimeFormat = time.RFC3339

// MetaData represents a single key-value pair for message metadata.
type MetaData struct {
	Key   string // Key is the identifier for the metadata entry.
	Value string // Value is the corresponding value for the metadata entry.
}

// Meta is a slice of MetaData items representing a collection of metadata entries.
type Meta []MetaData

// Map converts the Meta slice into a map[string]string.
// If duplicate keys exist, the last value for each key is kept.
//
// Example:
//
//	headers := meta.Map()
func (m Meta) Map() map[string]string {
	return maps.FromSliceWith(m, func(v MetaData) (string, string) { return v.Key, v.Value })
}

// All returns an iterator over the metadata entries.
//
// Example:
//
//	for key, value := range meta.All() { ... }
func (m Meta) All() iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		for _, v := range m {
			if !yield(v.Key, v.Value) {
				return
			}
		}
	}
}

// Value returns the value for the first metadata entry with the given key.
// Returns empty string and false if the key is not found.
//
// Example:
//
//	traceID, ok := meta.Value("trace_id")
func (m Meta) Value(key string) (string, bool) {
	for _, v := range m {
		if v.Key == key {
			return v.Value, true
		}
	}
	return "", false
}

// MetaFromMap converts a map[string]string into a Meta slice.
// Element order is not guaranteed due to map iteration.
//
// Example:
//
//	meta := msg.MetaFromMap(map[string]string{"trace_id": "abc123"})
func MetaFromMap(mm map[string]string) Meta {
	var meta = make(Meta, 0, len(mm))
	for k, v := range mm {
		meta = append(meta, MetaData{Key: k, Value: v})
	}
	return meta
}

// MessageCreatedTimeFromMeta extracts and parses the creation time from metadata.
// Returns zero time.Time and nil error if metadata is nil or key is not found.
//
// Example:
//
//	createdAt, err := msg.MessageCreatedTimeFromMeta(m.Metadata)
func MessageCreatedTimeFromMeta(meta Meta) (time.Time, error) {
	if meta == nil {
		return time.Time{}, nil
	}

	if v, ok := meta.Value(MetaKeyMessageCreatedTime); ok {
		return time.Parse(MessageCreatedTimeFormat, v)
	}

	return time.Time{}, nil
}

// MessageIdFromMeta extracts the message ID from metadata.
// Returns empty string if metadata is nil or key is not found.
//
// Example:
//
//	msgID := msg.MessageIdFromMeta(m.Metadata)
func MessageIdFromMeta(meta Meta) string {
	if meta == nil {
		return ""
	}

	if v, ok := meta.Value(MetaKeyMessageId); ok {
		return v
	}
	return ""
}
