// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas/data/keyset"
)

const (
	// DefaultPageSize is the page size [FetchPage] uses when Query.Limit is
	// not positive.
	DefaultPageSize = 100

	// MaxPageSize caps the page size [FetchPage] requests from a storage.
	MaxPageSize = 10_000
)

// ErrPagingNotConfigured is returned by [FetchPage] when a query carries a
// page token but no [PageTokens] were provided.
var ErrPagingNotConfigured = errors.New("audit: paging is not configured")

// Cursor is a position in the total order of events: event time, truncated
// to milliseconds, then event ID. Every storage orders events this way.
type Cursor struct {
	// Timestamp is the event time; only its millisecond part is significant.
	Timestamp time.Time
	// ID is the event ID; it breaks ties between equal timestamps.
	ID string
}

// CursorOf returns the position of event.
func CursorOf(event *Event) Cursor {
	return Cursor{Timestamp: event.Timestamp, ID: event.ID}
}

// Compare orders c against other in ascending (timestamp milliseconds, ID)
// order, returning -1, 0 or +1.
func (c Cursor) Compare(other Cursor) int {
	a, b := c.Timestamp.UnixMilli(), other.Timestamp.UnixMilli()
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return strings.Compare(c.ID, other.ID)
}

// Follows reports whether event comes strictly after c in the given order.
func (c Cursor) Follows(event *Event, order SortOrder) bool {
	cmp := CursorOf(event).Compare(c)
	if order == SortOrderAsc {
		return cmp > 0
	}
	return cmp < 0
}

// Page is one page of query results.
type Page struct {
	// Events are the events of the page in the query's sort order.
	Events []*Event
	// Next is the token of the following page, empty on the last page.
	Next string
}

// PageTokens issues and resolves audit page tokens. A token carries the
// position of the last event of a page and is bound to the query's filter,
// sort order and subject, so it cannot be replayed under another filter or by
// another principal.
type PageTokens struct {
	codec *keyset.Codec
}

// NewPageTokens returns PageTokens signing with codec.
func NewPageTokens(codec *keyset.Codec) *PageTokens {
	return &PageTokens{codec: codec}
}

// Encode returns the token of the page following event under query.
func (p *PageTokens) Encode(event *Event, query *Query) (string, error) {
	c := CursorOf(event)
	payload := strconv.AppendInt(nil, c.Timestamp.UnixMilli(), 10)
	payload = append(payload, '|')
	payload = append(payload, c.ID...)

	token, err := p.codec.Issue(payload, bindings(query))
	if err != nil {
		return "", fmt.Errorf("encode audit page token: %w", err)
	}
	return token, nil
}

// Decode resolves token under query. It fails with the [keyset] errors when
// the token is invalid, expired, or issued for another filter, sort order or
// subject.
func (p *PageTokens) Decode(token string, query *Query) (*Cursor, error) {
	payload, err := p.codec.Resolve(token, bindings(query))
	if err != nil {
		return nil, fmt.Errorf("decode audit page token: %w", err)
	}

	millis, id, found := strings.Cut(string(payload), "|")
	ms, parseErr := strconv.ParseInt(millis, 10, 64)
	if !found || parseErr != nil {
		return nil, fmt.Errorf("decode audit page token: %w", keyset.ErrInvalidToken)
	}
	return &Cursor{Timestamp: time.UnixMilli(ms), ID: id}, nil
}

// FetchPage returns the page of storage results for query: up to
// query.Limit events (DefaultPageSize when not positive, at most
// MaxPageSize) starting after query.After, and the token of the next page. A
// query with After requires tokens; Next is issued only when tokens is not
// nil and more events may follow.
//
// Continuation is keyset-based, not a snapshot: events inserted ahead of the
// position appear on later pages, events inserted behind it do not.
//
// Each (timestamp, ID) key is returned at most once. At-least-once delivery
// can store an event twice, and a storage that does not deduplicate on read
// (memory, ClickHouse before its parts merge, unless reads use FINAL)
// returns both copies next to each other, since they share the key. The
// copies are collapsed here, so a page can hold fewer events than the limit
// while Next is still issued; the cursor resumes strictly after the key, so
// a copy beyond the page boundary is skipped as well.
func FetchPage(ctx context.Context, storage Storage, tokens *PageTokens, query Query) (Page, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = DefaultPageSize
	}
	limit = min(limit, MaxPageSize)

	query.Cursor = nil
	if query.After != "" {
		if tokens == nil {
			return Page{}, ErrPagingNotConfigured
		}
		cursor, err := tokens.Decode(query.After, &query)
		if err != nil {
			return Page{}, err
		}
		query.Cursor = cursor
	}

	fetch := query
	fetch.Limit = limit + 1

	var (
		fetched int
		last    Cursor
	)
	events := make([]*Event, 0, min(limit+1, DefaultPageSize+1))
	for event, err := range storage.Query(ctx, &fetch) {
		if err != nil {
			return Page{}, err
		}
		fetched++
		key := CursorOf(event)
		if len(events) > 0 && key.Compare(last) == 0 {
			continue
		}
		last = key
		events = append(events, event)
	}

	// Fewer rows than requested: the storage has nothing beyond them.
	if fetched <= limit || tokens == nil {
		return Page{Events: events[:min(len(events), limit)]}, nil
	}

	events = events[:min(len(events), limit)]
	next, err := tokens.Encode(events[len(events)-1], &query)
	if err != nil {
		return Page{}, err
	}
	return Page{Events: events, Next: next}, nil
}

// bindings returns the token bindings of query: the filter excludes the page
// position and size, the sort is the effective order.
func bindings(q *Query) keyset.Bindings {
	return keyset.Bindings{
		Sort:    string(effectiveOrder(q.SortOrder)),
		Filter:  filterFingerprint(q),
		Subject: q.Subject,
	}
}

// effectiveOrder returns the order a storage applies: descending unless
// ascending is requested.
func effectiveOrder(o SortOrder) SortOrder {
	if o == SortOrderAsc {
		return SortOrderAsc
	}
	return SortOrderDesc
}

// filterFingerprint canonically encodes the filter fields of q. It appends
// into one buffer: the token path runs it on every page.
func filterFingerprint(q *Query) string {
	b := make([]byte, 0, fingerprintCap)
	field := func(name, value string) {
		b = append(b, name...)
		b = append(b, '=')
		b = strconv.AppendQuote(b, value)
		b = append(b, ';')
	}
	timeField := func(name string, t *time.Time) {
		if t != nil {
			b = append(b, name...)
			b = append(b, "=\""...)
			b = strconv.AppendInt(b, t.UnixNano(), 10)
			b = append(b, "\";"...)
		}
	}
	timeField("start", q.StartTime)
	timeField("end", q.EndTime)
	field("actor", q.ActorID)
	field("actorType", q.ActorType)
	field("resourceType", q.ResourceType)
	field("resource", q.ResourceID)
	field("type", string(q.EventType))
	field("action", string(q.Action))
	field("status", string(q.Status))
	field("request", q.RequestID)
	field("trace", q.TraceID)
	return string(b)
}

// fingerprintCap fits the field names, separators and short values of a
// typical filter, so the buffer rarely grows.
const fingerprintCap = 256
