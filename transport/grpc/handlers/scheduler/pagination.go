// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package scheduler

import (
	"errors"
	"fmt"

	"github.com/altessa-s/go-aip/pagination/pagetoken"

	horai "github.com/altessa-s/go-horai"
)

const (
	// DefaultPageSize is the number of items returned when no limit is specified.
	DefaultPageSize = 100

	// MaxPageSize is the maximum allowed page size.
	MaxPageSize = 1000
)

var (
	// ErrFilterParserRequired is returned by [New] without a filter parser.
	ErrFilterParserRequired = errors.New("scheduler filter parser is required")

	// ErrInvalidCursor is returned (as InvalidArgument) for a page token that
	// is malformed, tampered with, expired, or issued for another list, task
	// or filter expression.
	ErrInvalidCursor = errors.New("invalid or expired pagination cursor")

	// ErrPageTokensNotConfigured is returned (as FailedPrecondition) by List
	// and ListHistory of a Handler built without a page-token codec.
	ErrPageTokensNotConfigured = errors.New("scheduler: page tokens are not configured")

	// ErrSchedulerRequired is returned by [New] without a scheduler.
	ErrSchedulerRequired = errors.New("scheduler is required")
)

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

// pageSize clamps a requested page size.
func pageSize(requested int64) int {
	return pagetoken.PageSize(requested, DefaultPageSize, MaxPageSize)
}

// tasksBindings binds a task-list token to the filter expression.
func tasksBindings(filterExpr string) pagetoken.Bindings {
	return pagetoken.NewBindings(tasksSubject, tasksSort, filterExpr)
}

// historyBindings binds a history-list token to the task and the filter
// expression.
func historyBindings(taskID, filterExpr string) pagetoken.Bindings {
	return pagetoken.NewBindings(historySubject, historySort, taskID, filterExpr)
}

// encodeTaskCursor issues the token of the page after next, or nil at the end.
func (h *Handler) encodeTaskCursor(next *horai.TaskCursor, filterExpr string) (*string, error) {
	if next == nil {
		return nil, nil //nolint:nilnil // no next page
	}
	token, err := pagetoken.NewTyped[taskCursor](h.tokens).Issue(taskCursor{LastID: next.AfterID}, tasksBindings(filterExpr))
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// decodeTaskCursor verifies token against the filter expression; an empty
// token is the first page.
func (h *Handler) decodeTaskCursor(token, filterExpr string) (*horai.TaskCursor, error) {
	cur, first, err := pagetoken.NewTyped[taskCursor](h.tokens).Resolve(token, tasksBindings(filterExpr))
	if err != nil {
		return nil, invalidCursor(err)
	}
	if first {
		return nil, nil //nolint:nilnil // first page
	}
	return &horai.TaskCursor{AfterID: cur.LastID}, nil
}

// encodeHistoryCursor issues the token of the page after next, or nil at the
// end.
func (h *Handler) encodeHistoryCursor(next *horai.HistoryCursor, taskID, filterExpr string) (*string, error) {
	if next == nil {
		return nil, nil //nolint:nilnil // no next page
	}
	token, err := pagetoken.NewTyped[historyCursor](h.tokens).Issue(
		historyCursor{LastStartedAt: next.AfterStartedAt, LastID: next.AfterID}, historyBindings(taskID, filterExpr))
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// decodeHistoryCursor verifies token against the task and the filter
// expression; an empty token is the first page.
func (h *Handler) decodeHistoryCursor(token, taskID, filterExpr string) (*horai.HistoryCursor, error) {
	cur, first, err := pagetoken.NewTyped[historyCursor](h.tokens).Resolve(token, historyBindings(taskID, filterExpr))
	if err != nil {
		return nil, invalidCursor(err)
	}
	if first {
		return nil, nil //nolint:nilnil // first page
	}
	return &horai.HistoryCursor{AfterStartedAt: cur.LastStartedAt, AfterID: cur.LastID}, nil
}

// invalidCursor wraps a page-token failure with [ErrInvalidCursor].
func invalidCursor(err error) error {
	return fmt.Errorf("%w: %w", ErrInvalidCursor, err)
}
