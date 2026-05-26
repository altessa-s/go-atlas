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

	// TotalHits is the estimated total number of matching documents
	// reported by Meilisearch.
	TotalHits int64
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
	// filter expression. Returns task UID.
	DeleteDocumentsByFilter(ctx context.Context, indexName, filter string) (int64, error)

	// GetAllDocumentIDs paginates through the index and returns every primary key.
	GetAllDocumentIDs(ctx context.Context, indexName string) ([]string, error)

	// Search runs a full-text query against the specified index.
	Search(ctx context.Context, req *SearchRequest) (*SearchResult, error)
}
