// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/altessa-s/go-atlas/core/encoding/hash"
)

const (
	// DefaultPageSize is the number of items returned when no limit is specified.
	DefaultPageSize int64 = 100

	// MaxPageSize is the maximum allowed page size.
	MaxPageSize int64 = 1000
)

// ErrInvalidCursor is returned when a cursor token cannot be decoded or its
// filter hash does not match the current request's filter expression.
var ErrInvalidCursor = errors.New("invalid or expired pagination cursor")

// Pagination carries decoded cursor state and limit for storage backends.
// The filter AST is passed separately so backends clearly see what is
// pagination and what is filtering.
type Pagination struct {
	AfterID string // Last seen ID (from decoded cursor); empty = first page
	Limit   int64  // Clamped page size (storage should fetch Limit+1 for lookahead)
}

// HistoryPagination extends Pagination with history-specific cursor fields.
type HistoryPagination struct {
	Pagination
	AfterStartedAt int64 // StartedAt of the last seen entry (descending sort key)
}

// PageRequest carries pagination parameters for paginated list methods.
type PageRequest struct {
	Limit  int64
	Cursor string
}

// ClampLimit normalizes the limit to the range [1, MaxPageSize].
// A zero or negative value is replaced with [DefaultPageSize].
func (p PageRequest) ClampLimit() int64 {
	if p.Limit <= 0 {
		return DefaultPageSize
	}
	if p.Limit > MaxPageSize {
		return MaxPageSize
	}
	return p.Limit
}

// PageResult holds a single page of results along with an optional cursor to
// the next page. NextCursor is nil when there are no more results.
type PageResult[T any] struct {
	Items      []T
	NextCursor *string
}

// taskCursor is the JSON-encoded payload inside a task-list pagination token.
type taskCursor struct {
	LastID     string `json:"id"`
	FilterHash string `json:"fh"`
}

// historyCursor is the JSON-encoded payload inside a history-list pagination token.
type historyCursor struct {
	LastStartedAt int64  `json:"sa"`
	LastID        string `json:"id"`
	FilterHash    string `json:"fh"`
}

// filterHash computes a deterministic hash of a filter expression for cursor
// validation. An empty filter produces a fixed hash so that cursors remain
// valid across "no filter" requests.
func filterHash(filterExpr string) string {
	return hash.SHA256HexString(filterExpr)
}

// encodeTaskCursor produces a base64url-encoded cursor token from the last
// task ID and the current filter expression.
func encodeTaskCursor(lastID, filterExpr string) string {
	c := taskCursor{
		LastID:     lastID,
		FilterHash: filterHash(filterExpr),
	}
	b, _ := json.Marshal(c) //nolint:errcheck // struct of strings cannot fail
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeTaskCursor decodes a cursor token and validates its filter hash.
// Returns ErrInvalidCursor if the token is malformed or the filter has changed.
func decodeTaskCursor(token, filterExpr string) (*taskCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	var c taskCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, ErrInvalidCursor
	}
	if c.FilterHash != filterHash(filterExpr) {
		return nil, ErrInvalidCursor
	}
	return &c, nil
}

// encodeHistoryCursor produces a base64url-encoded cursor token from the last
// history entry's StartedAt, ID, and the current filter expression.
func encodeHistoryCursor(lastStartedAt int64, lastID, filterExpr string) string {
	c := historyCursor{
		LastStartedAt: lastStartedAt,
		LastID:        lastID,
		FilterHash:    filterHash(filterExpr),
	}
	b, _ := json.Marshal(c) //nolint:errcheck // simple struct cannot fail
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeHistoryCursor decodes a cursor token and validates its filter hash.
// Returns ErrInvalidCursor if the token is malformed or the filter has changed.
func decodeHistoryCursor(token, filterExpr string) (*historyCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	var c historyCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, ErrInvalidCursor
	}
	if c.FilterHash != filterHash(filterExpr) {
		return nil, ErrInvalidCursor
	}
	return &c, nil
}
