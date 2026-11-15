// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"context"
	"errors"
	"iter"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/pagination/pagetoken"

	"github.com/altessa-s/go-atlas/audit"
	"github.com/altessa-s/go-atlas/audit/storages/memory"
)

func newPageTokens(t *testing.T) *audit.PageTokens {
	t.Helper()
	codec, err := pagetoken.New([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	return audit.NewPageTokens(codec)
}

var pageBase = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

var _ audit.TokenCodec = (*pagetoken.Codec)(nil)

type tokenCodecFake struct {
	issue   func([]byte, pagetoken.Bindings) (string, error)
	resolve func(string, pagetoken.Bindings) ([]byte, error)
}

func (f tokenCodecFake) Issue(payload []byte, bindings pagetoken.Bindings) (string, error) {
	return f.issue(payload, bindings)
}

func (f tokenCodecFake) Resolve(token string, bindings pagetoken.Bindings) ([]byte, error) {
	return f.resolve(token, bindings)
}

func TestPageTokens_UsesCodecPort(t *testing.T) {
	t.Parallel()

	event := &audit.Event{ID: "event-1", Timestamp: pageBase}
	bindingsSeen := false
	tokens := audit.NewPageTokens(tokenCodecFake{
		issue: func(payload []byte, b pagetoken.Bindings) (string, error) {
			bindingsSeen = b.Subject == "user-1" && string(payload) == strconv.FormatInt(pageBase.UnixMilli(), 10)+"|event-1"
			return "fake-token", nil
		},
		resolve: func(token string, b pagetoken.Bindings) ([]byte, error) {
			require.Equal(t, "fake-token", token)
			require.True(t, bindingsSeen)
			return []byte(strconv.FormatInt(pageBase.UnixMilli(), 10) + "|event-1"), nil
		},
	})
	query := &audit.Query{Subject: "user-1"}
	token, err := tokens.Encode(event, query)
	require.NoError(t, err)
	require.Equal(t, "fake-token", token)
	cursor, err := tokens.Decode(token, query)
	require.NoError(t, err)
	require.Equal(t, event.ID, cursor.ID)
	require.True(t, event.Timestamp.Equal(cursor.Timestamp))

	boom := errors.New("codec unavailable")
	failing := audit.NewPageTokens(tokenCodecFake{
		issue:   func([]byte, pagetoken.Bindings) (string, error) { return "", boom },
		resolve: func(string, pagetoken.Bindings) ([]byte, error) { return nil, boom },
	})
	_, err = failing.Encode(event, query)
	require.ErrorIs(t, err, boom)
	_, err = failing.Decode("bad", query)
	require.ErrorIs(t, err, boom)
}

func seededStorage(t *testing.T, n int) audit.Storage {
	t.Helper()
	store := memory.New()
	events := make([]*audit.Event, n)
	for i := range n {
		events[i] = &audit.Event{
			ID:        "ev-" + strconv.Itoa(i),
			Type:      audit.EventTypeSystem,
			Action:    audit.ActionCreate,
			Actor:     audit.Actor{Type: audit.ActorTypeUser, ID: "u1"},
			Timestamp: pageBase.Add(time.Duration(i) * time.Second),
		}
	}
	require.NoError(t, store.StoreBatch(t.Context(), events))
	return store
}

func ids(events []*audit.Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.ID
	}
	return out
}

func TestCursor_CompareAndFollows(t *testing.T) {
	t.Parallel()

	a := audit.Cursor{Timestamp: pageBase, ID: "a"}
	b := audit.Cursor{Timestamp: pageBase, ID: "b"}
	later := audit.Cursor{Timestamp: pageBase.Add(time.Millisecond), ID: "a"}
	sameMilli := audit.Cursor{Timestamp: pageBase.Add(time.Microsecond), ID: "a"}

	require.Equal(t, -1, a.Compare(b))
	require.Equal(t, 1, b.Compare(a))
	require.Equal(t, -1, b.Compare(later), "timestamp dominates the ID")
	require.Equal(t, 1, later.Compare(b))
	require.Zero(t, a.Compare(sameMilli), "sub-millisecond differences are ignored")

	ev := &audit.Event{ID: "b", Timestamp: pageBase}
	require.Equal(t, b, audit.CursorOf(ev))
	require.True(t, a.Follows(ev, audit.SortOrderAsc))
	require.False(t, a.Follows(ev, audit.SortOrderDesc))
	require.False(t, a.Follows(ev, ""), "empty order is descending")
	require.True(t, later.Follows(ev, audit.SortOrderDesc))
	require.False(t, b.Follows(ev, audit.SortOrderAsc), "the cursor position itself does not follow")
}

func TestPageTokens_RoundTrip(t *testing.T) {
	t.Parallel()

	tokens := newPageTokens(t)
	start := pageBase.Add(-time.Hour)
	q := &audit.Query{StartTime: &start, ActorID: "u1", SortOrder: audit.SortOrderAsc, Subject: "alice"}
	ev := &audit.Event{ID: "x|y", Timestamp: pageBase.Add(1234 * time.Microsecond)}

	token, err := tokens.Encode(ev, q)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	cur, err := tokens.Decode(token, q)
	require.NoError(t, err)
	require.Equal(t, "x|y", cur.ID)
	require.Equal(t, pageBase.Add(time.Millisecond).UnixMilli(), cur.Timestamp.UnixMilli())
}

func TestPageTokens_DecodeRejectsChangedBindings(t *testing.T) {
	t.Parallel()

	tokens := newPageTokens(t)
	end := pageBase
	q := audit.Query{EndTime: &end, ResourceType: "doc", Subject: "alice"}
	token, err := tokens.Encode(&audit.Event{ID: "e", Timestamp: pageBase}, &q)
	require.NoError(t, err)

	otherFilter := q
	otherFilter.ResourceType = "user"
	_, err = tokens.Decode(token, &otherFilter)
	require.ErrorIs(t, err, pagetoken.ErrFilterChanged)

	otherSort := q
	otherSort.SortOrder = audit.SortOrderAsc
	_, err = tokens.Decode(token, &otherSort)
	require.ErrorIs(t, err, pagetoken.ErrSortChanged)

	explicitDesc := q
	explicitDesc.SortOrder = audit.SortOrderDesc
	_, err = tokens.Decode(token, &explicitDesc)
	require.NoError(t, err, "empty and desc are the same effective order")

	otherSubject := q
	otherSubject.Subject = "bob"
	_, err = tokens.Decode(token, &otherSubject)
	require.ErrorIs(t, err, pagetoken.ErrSubjectMismatch)

	_, err = tokens.Decode("garbage", &q)
	require.ErrorIs(t, err, pagetoken.ErrInvalidToken)
}

func TestFetchPage_PaginatesAscending(t *testing.T) {
	t.Parallel()

	store := seededStorage(t, 5)
	tokens := newPageTokens(t)
	q := audit.Query{Limit: 2, SortOrder: audit.SortOrderAsc, Subject: "alice"}

	var got []string
	pages := 0
	for {
		page, err := audit.FetchPage(t.Context(), store, tokens, q)
		require.NoError(t, err)
		pages++
		got = append(got, ids(page.Events)...)
		if page.Next == "" {
			break
		}
		require.Len(t, page.Events, 2)
		q.After = page.Next
	}
	require.Equal(t, 3, pages)
	require.Equal(t, []string{"ev-0", "ev-1", "ev-2", "ev-3", "ev-4"}, got)
}

func TestFetchPage_PaginatesDescendingByDefault(t *testing.T) {
	t.Parallel()

	store := seededStorage(t, 3)
	tokens := newPageTokens(t)

	page, err := audit.FetchPage(t.Context(), store, tokens, audit.Query{Limit: 2})
	require.NoError(t, err)
	require.Equal(t, []string{"ev-2", "ev-1"}, ids(page.Events))
	require.NotEmpty(t, page.Next)

	page, err = audit.FetchPage(t.Context(), store, tokens, audit.Query{Limit: 2, After: page.Next})
	require.NoError(t, err)
	require.Equal(t, []string{"ev-0"}, ids(page.Events))
	require.Empty(t, page.Next)
}

func TestFetchPage_WithoutTokensTruncatesWithoutNext(t *testing.T) {
	t.Parallel()

	page, err := audit.FetchPage(t.Context(), seededStorage(t, 4), nil, audit.Query{Limit: 2, SortOrder: audit.SortOrderAsc})
	require.NoError(t, err)
	require.Equal(t, []string{"ev-0", "ev-1"}, ids(page.Events))
	require.Empty(t, page.Next)
}

func TestFetchPage_AfterWithoutTokensFails(t *testing.T) {
	t.Parallel()

	_, err := audit.FetchPage(t.Context(), seededStorage(t, 1), nil, audit.Query{After: "tok"})
	require.ErrorIs(t, err, audit.ErrPagingNotConfigured)
}

func TestFetchPage_InvalidTokenFails(t *testing.T) {
	t.Parallel()

	_, err := audit.FetchPage(t.Context(), seededStorage(t, 1), newPageTokens(t), audit.Query{After: "not-a-token"})
	require.ErrorIs(t, err, pagetoken.ErrInvalidToken)
}

func TestFetchPage_TokenReplayUnderOtherFilterFails(t *testing.T) {
	t.Parallel()

	store := seededStorage(t, 3)
	tokens := newPageTokens(t)
	page, err := audit.FetchPage(t.Context(), store, tokens, audit.Query{Limit: 1, ActorID: "u1"})
	require.NoError(t, err)
	require.NotEmpty(t, page.Next)

	_, err = audit.FetchPage(t.Context(), store, tokens, audit.Query{Limit: 1, ActorID: "u2", After: page.Next})
	require.ErrorIs(t, err, pagetoken.ErrFilterChanged)
}

// scriptedStorage serves a fixed sequence of results regardless of the query.
type scriptedStorage struct {
	audit.Storage
	events []*audit.Event
	err    error
	limits []int
}

func (s *scriptedStorage) Query(_ context.Context, q *audit.Query) iter.Seq2[*audit.Event, error] {
	s.limits = append(s.limits, q.Limit)
	return func(yield func(*audit.Event, error) bool) {
		for _, e := range s.events {
			if !yield(e, nil) {
				return
			}
		}
		if s.err != nil {
			yield(nil, s.err)
		}
	}
}

func TestFetchPage_StorageErrorPropagates(t *testing.T) {
	t.Parallel()

	errBoom := errors.New("boom")
	st := &scriptedStorage{events: []*audit.Event{{ID: "a", Timestamp: pageBase}}, err: errBoom}
	_, err := audit.FetchPage(t.Context(), st, nil, audit.Query{})
	require.ErrorIs(t, err, errBoom)
	require.Equal(t, []int{audit.DefaultPageSize + 1}, st.limits, "one lookahead row beyond the default page size")
}

func TestFetchPage_CollapsesDuplicateKeys(t *testing.T) {
	t.Parallel()

	e1 := &audit.Event{ID: "a", Timestamp: pageBase}
	e1dup := &audit.Event{ID: "a", Timestamp: pageBase}
	e2 := &audit.Event{ID: "b", Timestamp: pageBase.Add(time.Second)}
	e3 := &audit.Event{ID: "c", Timestamp: pageBase.Add(2 * time.Second)}
	st := &scriptedStorage{events: []*audit.Event{e1, e1dup, e2}}
	tokens := newPageTokens(t)

	// limit 2 fetches 3 rows; the duplicate collapses so the page holds two
	// distinct events and, since the storage returned more than the limit,
	// a continuation token is issued after the last one.
	page, err := audit.FetchPage(t.Context(), st, tokens, audit.Query{Limit: 2, SortOrder: audit.SortOrderAsc})
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, ids(page.Events))
	require.NotEmpty(t, page.Next)

	cur, err := tokens.Decode(page.Next, &audit.Query{SortOrder: audit.SortOrderAsc})
	require.NoError(t, err)
	require.Equal(t, "b", cur.ID)

	st = &scriptedStorage{events: []*audit.Event{e1, e2, e3}}
	page, err = audit.FetchPage(t.Context(), st, tokens, audit.Query{Limit: 2, SortOrder: audit.SortOrderAsc})
	require.NoError(t, err)
	require.Equal(t, []string{"a", "b"}, ids(page.Events))
	require.NotEmpty(t, page.Next)
}
