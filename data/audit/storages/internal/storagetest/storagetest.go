// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package storagetest

import (
	"bytes"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/audit"
	"github.com/altessa-s/go-atlas/data/keyset"
)

// NewStorage returns an empty storage for one test. Each call must return an
// isolated storage, since subtests run in parallel.
type NewStorage func(t *testing.T) audit.Storage

// Run checks that a storage implements the audit paging contract: the total
// (timestamp milliseconds, ID) order in both directions, continuation after
// a cursor, exact page boundaries, continuation under inserts, Count
// semantics, millisecond time-range bounds, replayed events and page token
// bindings.
//
// rejectsReplay recognizes the error a storage returns when it refuses an
// event already stored (a unique-key violation); nil means the storage must
// accept the replay. Any other error fails the Replay test.
func Run(t *testing.T, newStorage NewStorage, rejectsReplay func(error) bool) {
	t.Helper()

	t.Run("Order", func(t *testing.T) { t.Parallel(); testOrder(t, newStorage) })
	t.Run("PageBoundaries", func(t *testing.T) { t.Parallel(); testPageBoundaries(t, newStorage) })
	t.Run("InsertsDuringPaging", func(t *testing.T) { t.Parallel(); testInsertsDuringPaging(t, newStorage) })
	t.Run("Count", func(t *testing.T) { t.Parallel(); testCount(t, newStorage) })
	t.Run("TimeRange", func(t *testing.T) { t.Parallel(); testTimeRange(t, newStorage) })
	t.Run("Replay", func(t *testing.T) { t.Parallel(); testReplay(t, newStorage, rejectsReplay) })
	t.Run("TokenBindings", func(t *testing.T) { t.Parallel(); testTokenBindings(t, newStorage) })
}

var base = time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

func event(id string, offset time.Duration, actor string) *audit.Event {
	return &audit.Event{
		ID:        id,
		Type:      audit.EventType("test"),
		Action:    audit.Action("read"),
		Timestamp: base.Add(offset),
		Actor:     audit.Actor{ID: actor, Type: "user"},
		Resource:  audit.Resource{Type: "doc", ID: "r-" + id},
		Result:    audit.Result{Status: audit.ResultStatus("success")},
	}
}

func tokens(t *testing.T) *audit.PageTokens {
	t.Helper()
	codec, err := keyset.New(bytes.Repeat([]byte{7}, keyset.MinKeyLength))
	require.NoError(t, err)
	return audit.NewPageTokens(codec)
}

func store(t *testing.T, s audit.Storage, events ...*audit.Event) {
	t.Helper()
	require.NoError(t, s.StoreBatch(t.Context(), events))
}

func ids(events []*audit.Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.ID
	}
	return out
}

// pageAll walks every page of query and returns the IDs in order.
//
//nolint:mnd // Fixed timestamps, page sizes and counts describe the storage contract.
func pageAll(t *testing.T, s audit.Storage, pt *audit.PageTokens, query audit.Query) []string {
	t.Helper()
	var out []string
	for range 100 {
		page, err := audit.FetchPage(t.Context(), s, pt, query)
		require.NoError(t, err)
		out = append(out, ids(page.Events)...)
		if page.Next == "" {
			return out
		}
		query.After = page.Next
	}
	t.Fatal("paging did not terminate")
	return nil
}

// Events stored out of order, several sharing a millisecond (sub-millisecond
// differences are not significant) and ordered there by ID.
//
//nolint:mnd // Fixed timestamps describe the storage contract.
func orderFixture() []*audit.Event {
	return []*audit.Event{
		event("e", 2*time.Millisecond, "u1"),
		event("b", time.Millisecond+300*time.Microsecond, "u1"),
		event("a", 0, "u1"),
		event("d", time.Millisecond, "u1"),
		event("c", time.Millisecond+700*time.Microsecond, "u1"),
	}
}

//nolint:mnd // Fixed timestamps, page sizes and counts describe the storage contract.
func testOrder(t *testing.T, newStorage NewStorage) {
	s := newStorage(t)
	store(t, s, orderFixture()...)
	pt := tokens(t)

	asc := []string{"a", "b", "c", "d", "e"}
	desc := slices.Clone(asc)
	slices.Reverse(desc)

	for _, limit := range []int{1, 2, 3, 5, 10} {
		require.Equal(t, asc, pageAll(t, s, pt, audit.Query{Limit: limit, SortOrder: audit.SortOrderAsc}), "asc limit %d", limit)
		require.Equal(t, desc, pageAll(t, s, pt, audit.Query{Limit: limit}), "desc limit %d", limit)
	}
}

//nolint:mnd // Fixed timestamps, page sizes and counts describe the storage contract.
func testPageBoundaries(t *testing.T, newStorage NewStorage) {
	s := newStorage(t)
	events := make([]*audit.Event, 4)
	for i := range events {
		events[i] = event(fmt.Sprintf("e%d", i), time.Duration(i)*time.Second, "u1")
	}
	store(t, s, events...)
	pt := tokens(t)

	// Exactly two full pages: the second has no next token.
	page, err := audit.FetchPage(t.Context(), s, pt, audit.Query{Limit: 2, SortOrder: audit.SortOrderAsc})
	require.NoError(t, err)
	require.Equal(t, []string{"e0", "e1"}, ids(page.Events))
	require.NotEmpty(t, page.Next)

	page, err = audit.FetchPage(t.Context(), s, pt, audit.Query{Limit: 2, SortOrder: audit.SortOrderAsc, After: page.Next})
	require.NoError(t, err)
	require.Equal(t, []string{"e2", "e3"}, ids(page.Events))
	require.Empty(t, page.Next, "a full last page has no next token")

	// No tokens: a single page, never a next token.
	page, err = audit.FetchPage(t.Context(), s, nil, audit.Query{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page.Events, 2)
	require.Empty(t, page.Next)

	_, err = audit.FetchPage(t.Context(), s, nil, audit.Query{After: "x"})
	require.ErrorIs(t, err, audit.ErrPagingNotConfigured)
}

//nolint:mnd // Fixed timestamps, page sizes and counts describe the storage contract.
func testInsertsDuringPaging(t *testing.T, newStorage NewStorage) {
	s := newStorage(t)
	store(t, s, event("e1", time.Second, "u1"), event("e2", 2*time.Second, "u1"), event("e3", 3*time.Second, "u1"))
	pt := tokens(t)

	query := audit.Query{Limit: 1, SortOrder: audit.SortOrderAsc}
	page, err := audit.FetchPage(t.Context(), s, pt, query)
	require.NoError(t, err)
	require.Equal(t, []string{"e1"}, ids(page.Events))

	// One event behind the position, one ahead of it.
	store(t, s, event("e0", 0, "u1"), event("e4", 4*time.Second, "u1"))

	query.After = page.Next
	query.Limit = 10
	rest := pageAll(t, s, pt, query)
	require.Equal(t, []string{"e2", "e3", "e4"}, rest, "events ahead of the position appear; events behind it do not")
}

//nolint:mnd // Fixed timestamps, page sizes and counts describe the storage contract.
func testCount(t *testing.T, newStorage NewStorage) {
	s := newStorage(t)
	store(t, s, orderFixture()...)
	store(t, s, event("other", 0, "u2"))
	pt := tokens(t)

	page, err := audit.FetchPage(t.Context(), s, pt, audit.Query{Limit: 2, ActorID: "u1"})
	require.NoError(t, err)

	count, err := s.Count(t.Context(), &audit.Query{
		ActorID: "u1",
		Limit:   1,
		After:   page.Next,
		Cursor:  &audit.Cursor{Timestamp: base.Add(time.Hour), ID: "zzz"},
	})
	require.NoError(t, err)
	require.Equal(t, int64(5), count, "Count matches the filter and ignores the page position and size")
}

// Time-range bounds are compared at millisecond precision: a bound's
// sub-millisecond part selects nothing more and nothing less.
//
//nolint:mnd // Fixed timestamps, page sizes and counts describe the storage contract.
func testTimeRange(t *testing.T, newStorage NewStorage) {
	s := newStorage(t)
	store(t, s, orderFixture()...)
	pt := tokens(t)

	start := base.Add(time.Millisecond + 500*time.Microsecond)
	end := base.Add(time.Millisecond + 900*time.Microsecond)
	query := audit.Query{StartTime: &start, EndTime: &end, SortOrder: audit.SortOrderAsc}

	require.Equal(t, []string{"b", "c", "d"}, pageAll(t, s, pt, query), "events of millisecond 1 only")

	count, err := s.Count(t.Context(), &query)
	require.NoError(t, err)
	require.Equal(t, int64(3), count)
}

// A replayed event — the same ID and timestamp stored again, as
// at-least-once delivery does — is returned once by paging, whether its
// copies fall inside a page or across a page boundary. A storage that
// rejects the replay, as rejectsReplay recognizes, holds no copy and is not
// exercised further.
//
//nolint:mnd // Fixed timestamps, page sizes and counts describe the storage contract.
func testReplay(t *testing.T, newStorage NewStorage, rejectsReplay func(error) bool) {
	s := newStorage(t)
	events := make([]*audit.Event, 4)
	for i := range events {
		events[i] = event(fmt.Sprintf("e%d", i), time.Duration(i)*time.Second, "u1")
	}
	store(t, s, events...)
	for _, e := range []*audit.Event{events[1], events[3]} {
		if err := s.Store(t.Context(), e); err != nil {
			if rejectsReplay == nil || !rejectsReplay(err) {
				require.NoError(t, err, "replaying a stored event")
			}
			t.Skipf("storage rejects a replayed event: %v", err)
		}
	}
	pt := tokens(t)

	want := []string{"e0", "e1", "e2", "e3"}
	desc := slices.Clone(want)
	slices.Reverse(desc)
	for _, limit := range []int{1, 2, 3, 4, 10} {
		require.Equal(t, want, pageAll(t, s, pt, audit.Query{Limit: limit, SortOrder: audit.SortOrderAsc}), "asc limit %d", limit)
		require.Equal(t, desc, pageAll(t, s, pt, audit.Query{Limit: limit}), "desc limit %d", limit)
	}
}

//nolint:mnd // Fixed timestamps, page sizes and counts describe the storage contract.
func testTokenBindings(t *testing.T, newStorage NewStorage) {
	s := newStorage(t)
	store(t, s, orderFixture()...)
	pt := tokens(t)

	query := audit.Query{Limit: 1, ActorID: "u1", Subject: "alice"}
	page, err := audit.FetchPage(t.Context(), s, pt, query)
	require.NoError(t, err)
	require.NotEmpty(t, page.Next)

	for name, tc := range map[string]struct {
		mutate func(q *audit.Query)
		want   error
	}{
		"filter":  {func(q *audit.Query) { q.ActorID = "u2" }, keyset.ErrFilterChanged},
		"sort":    {func(q *audit.Query) { q.SortOrder = audit.SortOrderAsc }, keyset.ErrSortChanged},
		"subject": {func(q *audit.Query) { q.Subject = "mallory" }, keyset.ErrSubjectMismatch},
		"tamper":  {func(q *audit.Query) { q.After = tamper(q.After) }, keyset.ErrInvalidToken},
	} {
		q := query
		q.After = page.Next
		tc.mutate(&q)
		_, fetchErr := audit.FetchPage(t.Context(), s, pt, q)
		require.ErrorIs(t, fetchErr, tc.want, name)
	}

	// The limit is not bound: a token continues with another page size.
	q := query
	q.After, q.Limit = page.Next, 10
	_, err = audit.FetchPage(t.Context(), s, pt, q)
	require.NoError(t, err)
}

// tamper changes the first character of a token to another one of the
// alphabet.
func tamper(token string) string {
	c := byte('A')
	if token[0] == c {
		c = 'B'
	}
	return string(c) + token[1:]
}
