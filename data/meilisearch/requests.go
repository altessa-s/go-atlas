// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"context"
	"encoding/json"
)

// SearchRequest holds parameters for a full-text search query.
type SearchRequest struct {
	// IndexName specifies which Meilisearch index to query.
	IndexName string

	// Query is the full-text search query string.
	Query string

	// Filter is an optional Meilisearch filter expression.
	//
	// SECURITY: see [Client.DeleteDocumentsByFilter] for the
	// filter-injection considerations — the same caveats apply when
	// constructing Filter from user input.
	Filter string

	// Sort specifies sorting rules in Meilisearch DSL (e.g. "created_at:desc").
	Sort []string

	// Offset is the zero-based offset of the first hit to return.
	Offset int64

	// Limit caps the number of hits returned.
	Limit int64
}

// SearchResult holds the response of a [Client.Search] call.
type SearchResult struct {
	// Hits contains raw JSON documents so callers can deserialize into any
	// target type without coupling this package to domain models.
	Hits []json.RawMessage

	// EstimatedTotalHits is Meilisearch's estimate of the total number of
	// matching documents. The "Estimated" prefix is intentional and
	// matches the SDK field name: Meilisearch trades exact counts for
	// search latency, so this number can drift slightly between
	// invocations even for an unchanged corpus. Treat it as an
	// approximation, not a database count.
	EstimatedTotalHits int64
}

// FetchResult holds the response of a [Client.FetchDocuments] call.
type FetchResult struct {
	// Hits contains raw JSON documents so callers can deserialize into any
	// target type without coupling this package to domain models.
	Hits []json.RawMessage

	// Total is the total number of documents matching the filter (across
	// all pages), as reported by Meilisearch. Use it to decide whether
	// another page is worth fetching instead of relying on a short-page
	// signal — the latter is ambiguous when Total is an exact multiple of
	// Limit.
	Total int64
}

// MeilisearchClient is the surface domain code depends on. [Client] implements it.
type MeilisearchClient interface {
	// IndexDocuments adds or updates documents in indexName. Returns task UID.
	IndexDocuments(ctx context.Context, indexName string, documents any) (int64, error)

	// DeleteDocument removes a single document by ID. Returns task UID.
	DeleteDocument(ctx context.Context, indexName, documentID string) (int64, error)

	// DeleteDocuments removes multiple documents by ID. Returns task UID.
	DeleteDocuments(ctx context.Context, indexName string, documentIDs []string) (int64, error)

	// DeleteDocumentsByFilter removes documents matching the Meilisearch
	// filter expression. Returns task UID. See [Client.DeleteDocumentsByFilter]
	// for the filter-injection caveat.
	DeleteDocumentsByFilter(ctx context.Context, indexName, filter string) (int64, error)

	// FetchDocuments returns a page of raw documents matching the optional
	// Meilisearch filter. See [Client.FetchDocuments] for the
	// filter-injection caveat.
	FetchDocuments(ctx context.Context, indexName, filter string, offset, limit int64) (*FetchResult, error)

	// GetAllDocumentIDs paginates through the index and returns every primary key
	// (assumes the default "id" field; use [Client.GetAllDocumentIDsWithPrimaryKey]
	// directly when the index uses a different primary key).
	GetAllDocumentIDs(ctx context.Context, indexName string) ([]string, error)

	// Search runs a full-text query against the specified index.
	Search(ctx context.Context, req *SearchRequest) (*SearchResult, error)
}
