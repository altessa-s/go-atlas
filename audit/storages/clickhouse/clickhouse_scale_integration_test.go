// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package clickhouse_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/pagination/pagetoken"

	"github.com/altessa-s/go-atlas/audit"
	"github.com/altessa-s/go-atlas/audit/storages/storagetest"

	auditclickhouse "github.com/altessa-s/go-atlas/audit/storages/clickhouse"
)

func TestIntegration_Conformance(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, func(t *testing.T) audit.Storage {
		return newTestStorage(t, auditclickhouse.WithTimeRangeMode(auditclickhouse.TimeRangeModeDisabled))
	}, nil)
}

// Paging through a large table with small pages returns every event once, in
// order, including runs of events sharing a millisecond.
func TestIntegration_ScalePaging(t *testing.T) {
	t.Parallel()

	const total = 10_000
	s := newTestStorage(t, auditclickhouse.WithTimeRangeMode(auditclickhouse.TimeRangeModeDisabled))

	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	events := make([]*audit.Event, total)
	for i := range events {
		events[i] = &audit.Event{
			ID:        fmt.Sprintf("evt-%06d", total-i), // IDs against insertion order
			Type:      "scale",
			Action:    "write",
			Timestamp: base.Add(time.Duration(i/7) * time.Millisecond), // runs of 7 share a millisecond
			Actor:     audit.Actor{ID: "u1"},
		}
	}
	require.NoError(t, s.StoreBatch(t.Context(), events))

	codec, err := pagetoken.New(bytes.Repeat([]byte{9}, pagetoken.MinKeyLength))
	require.NoError(t, err)
	tokens := audit.NewPageTokens(codec)

	query := audit.Query{Limit: 250, SortOrder: audit.SortOrderAsc}
	var got []*audit.Event
	for range total {
		page, err := audit.FetchPage(t.Context(), s, tokens, query)
		require.NoError(t, err)
		got = append(got, page.Events...)
		if page.Next == "" {
			break
		}
		query.After = page.Next
	}

	require.Len(t, got, total)
	require.True(t, slices.IsSortedFunc(got, func(a, b *audit.Event) int {
		return audit.CursorOf(a).Compare(audit.CursorOf(b))
	}), "pages must follow the (timestamp, ID) order")

	seen := make(map[string]struct{}, total)
	for _, e := range got {
		seen[e.ID] = struct{}{}
	}
	require.Len(t, seen, total, "no event may repeat across pages")
}

func TestStoreBatchPartialFailure(t *testing.T) {
	t.Parallel()
	s := newTestStorage(t, auditclickhouse.WithMaxBatchSize(1))
	at := time.Now().UTC()
	good, bad := testEvent("before", at), testEvent("invalid", at)
	bad.Metadata = map[string]any{"unencodable": make(chan int)}
	err := storagetest.BatchFailure(t, s, []*audit.Event{good, bad, testEvent("after", at)}, []string{good.ID})
	var typeErr *json.UnsupportedTypeError
	require.ErrorAs(t, err, &typeErr)
}
