// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"context"
	"encoding/json"
	"testing"

	msdk "github.com/meilisearch/meilisearch-go"
)

// BenchmarkSearch_HitMarshaling measures the per-call cost of the
// hit-marshaling loop in Client.Search: SDK returns msdk.Hits (=
// []map[string]json.RawMessage); we re-marshal each hit into a single
// json.RawMessage so callers can decode into their own struct without
// double-decoding. This is the hot path for any search-heavy service
// (the SDK round-trip itself dominates wall-clock, but the in-process
// allocations show up under contention).
//
// Uses a sealed in-process fake SDK so the benchmark measures only the
// wrapper overhead, not network latency.
func BenchmarkSearch_HitMarshaling(b *testing.B) {
	const hitsPerPage = 20
	hits := make(msdk.Hits, 0, hitsPerPage)
	for i := range hitsPerPage {
		hits = append(hits, msdk.Hit{
			"id":          json.RawMessage(`"doc-` + itoa(i) + `"`),
			"title":       json.RawMessage(`"Document title ` + itoa(i) + `"`),
			"description": json.RawMessage(`"Lorem ipsum dolor sit amet, consectetur adipiscing elit."`),
		})
	}

	idx := &fakeIndex{
		searchFn: func(_ context.Context, _ string, _ *msdk.SearchRequest) (*msdk.SearchResponse, error) {
			return &msdk.SearchResponse{Hits: hits, EstimatedTotalHits: int64(hitsPerPage)}, nil
		},
	}
	sdk := &fakeSDK{indexFn: func(_ string) msdk.IndexManager { return idx }}
	c := newTestClient(sdk)
	req := &SearchRequest{IndexName: "things", Query: "lorem", Limit: hitsPerPage}
	ctx := context.Background()

	b.ReportAllocs()
	for b.Loop() {
		_, err := c.Search(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// itoa is a tiny helper to keep the benchmark setup loop free of
// strconv-imports; the SDK fake is wired with literal byte slices so
// b.Loop sees no allocations from the test scaffolding.
func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
