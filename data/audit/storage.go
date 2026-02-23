// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"context"
	"iter"
	"time"
)

// Storage defines the persistence interface for audit events.
type Storage interface {
	// Store persists a single audit event.
	Store(ctx context.Context, event *Event) error

	// StoreBatch persists multiple audit events atomically.
	StoreBatch(ctx context.Context, events []*Event) error

	// Query returns an iterator over events matching the given criteria.
	Query(ctx context.Context, query *Query) iter.Seq2[*Event, error]

	// Count returns the number of events matching the given criteria.
	Count(ctx context.Context, query *Query) (int64, error)

	// Close releases any resources held by the storage.
	Close(ctx context.Context) error
}

// Query defines filter criteria for querying audit events.
type Query struct {
	StartTime    *time.Time
	EndTime      *time.Time
	ActorID      string
	ActorType    string
	ResourceType string
	ResourceID   string
	EventType    EventType
	Action       Action
	Status       ResultStatus
	RequestID    string
	TraceID      string
	Limit        int
	Offset       int
	SortOrder    SortOrder
}
