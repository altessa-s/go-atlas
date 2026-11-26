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
	// Store persists a single audit event. A nil event is rejected with an error
	// matching go-atlas-core/errors.ErrInvalidArgument. Idempotent backends
	// (MongoDB, SQL) deduplicate by Event.ID only, so callers invoking Storage
	// directly without [Auditor] must supply unique IDs.
	Store(ctx context.Context, event *Event) error

	// StoreBatch persists multiple audit events. An empty batch succeeds. A nil
	// element rejects the whole batch before any write, with an error matching
	// go-atlas-core/errors.ErrInvalidArgument.
	//
	// Replay idempotency in MongoDB and SQL is keyed by Event.ID only. After a
	// write error, MongoDB unordered inserts can retain valid events from any
	// position in the batch and ClickHouse chunked inserts can leave completed
	// chunks persisted. Memory appends atomically. SQL commits atomically in a
	// storage-owned transaction; callers must roll back their own transaction on
	// error.
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
	SortOrder    SortOrder

	// After is the page token returned with the previous page by [FetchPage].
	// It is resolved by FetchPage and bound to the filter, the sort order and
	// Subject; storages read Cursor instead.
	After string

	// Subject identifies the principal paging through the results, typically
	// the authenticated caller. A page token issued to one subject is rejected
	// for another.
	Subject string

	// Cursor is the exclusive position to continue after, set by [FetchPage]
	// from After. Storages return only events strictly after it in the sort
	// order. Nil starts from the first event.
	Cursor *Cursor
}
