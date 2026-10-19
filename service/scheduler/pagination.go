// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/altessa-s/go-atlas/data/keyset"
)

const (
	// DefaultPageSize is the number of items returned when no limit is specified.
	DefaultPageSize int64 = 100

	// MaxPageSize is the maximum allowed page size.
	MaxPageSize int64 = 1000
)

// ErrInvalidCursor is returned when a cursor token fails verification: it is
// malformed, tampered with, expired, signed with another key, or was issued
// for another list, task or filter expression. The [keyset] error is wrapped
// as well, so [keyset.Classify] reports the failure.
var ErrInvalidCursor = errors.New("invalid or expired pagination cursor")

// ErrPageTokensNotConfigured is returned by [Scheduler.TasksPaginated] and
// [Scheduler.HistoryPaginated] when the scheduler has no page-token codec:
// configure one shared by every instance with [WithPageTokens] (factory
// UsePageTokens or the `paging` config section), or opt in to a per-instance
// random key with [WithEphemeralPageTokens].
var ErrPageTokensNotConfigured = errors.New("scheduler: page tokens are not configured")

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
	return int64(keyset.PageSize(p.Limit, int(DefaultPageSize), int(MaxPageSize)))
}

// PageResult holds a single page of results along with an optional cursor to
// the next page. NextCursor is nil when there are no more results.
type PageResult[T any] struct {
	Items      []T
	NextCursor *string
}

// Subjects and sorts the page tokens of the paginated lists are bound to.
const (
	tasksSubject   = "scheduler.TasksPaginated"
	tasksSort      = "id:1"
	historySubject = "scheduler.HistoryPaginated"
	historySort    = "startedAt:-1,id:-1"
)

// taskCursor is the position inside a task-list page token.
type taskCursor struct {
	LastID string `json:"id"`
}

// historyCursor is the position inside a history-list page token.
type historyCursor struct {
	LastStartedAt int64  `json:"sa"`
	LastID        string `json:"id"`
}

// newEphemeralPageTokens returns a codec signing with a random key, for
// [WithEphemeralPageTokens]: its tokens verify only in this process.
func newEphemeralPageTokens() *keyset.Codec {
	key := make([]byte, keyset.MinKeyLength)
	_, _ = rand.Read(key)       //nolint:errcheck // crypto/rand.Read never fails
	codec, _ := keyset.New(key) //nolint:errcheck // the key has the minimum length
	return codec
}

// tasksBindings binds a task-list token to the filter expression.
func tasksBindings(filterExpr string) keyset.Bindings {
	return keyset.NewBindings(tasksSubject, tasksSort, filterExpr)
}

// historyBindings binds a history-list token to the task and the filter
// expression.
func historyBindings(taskID, filterExpr string) keyset.Bindings {
	return keyset.NewBindings(historySubject, historySort, taskID, filterExpr)
}

// invalidCursor wraps a page-token failure with [ErrInvalidCursor].
func invalidCursor(err error) error {
	return fmt.Errorf("%w: %w", ErrInvalidCursor, err)
}

// encodeTaskCursor returns the token of the task page after lastID.
func (s *Scheduler) encodeTaskCursor(lastID, filterExpr string) (string, error) {
	return keyset.NewTyped[taskCursor](s.opts.pageTokens).Issue(taskCursor{LastID: lastID}, tasksBindings(filterExpr))
}

// decodeTaskCursor verifies token against the filter expression. It returns
// [ErrInvalidCursor] for a token that fails verification.
func (s *Scheduler) decodeTaskCursor(token, filterExpr string) (taskCursor, error) {
	cur, _, err := keyset.NewTyped[taskCursor](s.opts.pageTokens).Resolve(token, tasksBindings(filterExpr))
	if err != nil {
		return taskCursor{}, invalidCursor(err)
	}
	return cur, nil
}

// encodeHistoryCursor returns the token of the history page after the entry
// that started at lastStartedAt with lastID.
func (s *Scheduler) encodeHistoryCursor(taskID string, lastStartedAt int64, lastID, filterExpr string) (string, error) {
	return keyset.NewTyped[historyCursor](s.opts.pageTokens).Issue(
		historyCursor{LastStartedAt: lastStartedAt, LastID: lastID}, historyBindings(taskID, filterExpr))
}

// decodeHistoryCursor verifies token against the task and the filter
// expression. It returns [ErrInvalidCursor] for a token that fails
// verification.
func (s *Scheduler) decodeHistoryCursor(token, taskID, filterExpr string) (historyCursor, error) {
	cur, _, err := keyset.NewTyped[historyCursor](s.opts.pageTokens).Resolve(token, historyBindings(taskID, filterExpr))
	if err != nil {
		return historyCursor{}, invalidCursor(err)
	}
	return cur, nil
}
