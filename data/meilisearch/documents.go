// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	msdk "github.com/meilisearch/meilisearch-go"
)

// documentIDListBatchSize is the page size [Client.GetAllDocumentIDs] uses
// when paginating through an index.
const documentIDListBatchSize int64 = 1000

// documentIDFields restricts [Client.GetAllDocumentIDs] responses to the
// primary key only, so the full document body is not transferred.
var documentIDFields = []string{"id"}

// IndexDocuments adds or updates documents in indexName. documents must be a
// slice (or array) of structs/maps that include the index's primary key.
// Returns the Meilisearch task UID for tracking the async operation.
func (c *Client) IndexDocuments(ctx context.Context, indexName string, documents any) (int64, error) {
	task, err := c.sdk.Index(indexName).AddDocumentsWithContext(ctx, documents, nil)
	if err != nil {
		return 0, fmt.Errorf("index documents in %s: %w", indexName, err)
	}

	c.logger.Debug("documents indexed",
		slog.String("index", indexName),
		slog.Int64("task_uid", task.TaskUID))

	return task.TaskUID, nil
}

// DeleteDocument removes a single document by ID. Returns task UID.
func (c *Client) DeleteDocument(ctx context.Context, indexName, documentID string) (int64, error) {
	task, err := c.sdk.Index(indexName).DeleteDocumentWithContext(ctx, documentID, nil)
	if err != nil {
		return 0, fmt.Errorf("delete document %s from %s: %w", documentID, indexName, err)
	}

	c.logger.Debug("document deleted",
		slog.String("index", indexName),
		slog.String("document_id", documentID),
		slog.Int64("task_uid", task.TaskUID))

	return task.TaskUID, nil
}

// DeleteDocuments removes multiple documents by ID. Returns task UID.
func (c *Client) DeleteDocuments(ctx context.Context, indexName string, documentIDs []string) (int64, error) {
	task, err := c.sdk.Index(indexName).DeleteDocumentsWithContext(ctx, documentIDs, nil)
	if err != nil {
		return 0, fmt.Errorf("delete documents from %s: %w", indexName, err)
	}

	c.logger.Debug("documents deleted",
		slog.String("index", indexName),
		slog.Int("count", len(documentIDs)),
		slog.Int64("task_uid", task.TaskUID))

	return task.TaskUID, nil
}

// DeleteDocumentsByFilter removes documents matching the given Meilisearch
// filter expression. Returns task UID.
func (c *Client) DeleteDocumentsByFilter(ctx context.Context, indexName, filter string) (int64, error) {
	task, err := c.sdk.Index(indexName).DeleteDocumentsByFilterWithContext(ctx, filter, nil)
	if err != nil {
		return 0, fmt.Errorf("delete documents by filter from %s: %w", indexName, err)
	}

	c.logger.Debug("documents deleted by filter",
		slog.String("index", indexName),
		slog.String("filter", filter),
		slog.Int64("task_uid", task.TaskUID))

	return task.TaskUID, nil
}

// GetAllDocumentIDs paginates through indexName and returns every document's
// primary key. Intended for sync/reconciliation flows; pulls
// [documentIDListBatchSize] IDs per round-trip and reads only the "id" field.
func (c *Client) GetAllDocumentIDs(ctx context.Context, indexName string) ([]string, error) {
	index := c.sdk.Index(indexName)

	var (
		allIDs []string
		offset int64
	)

	for {
		var result msdk.DocumentsResult
		err := index.GetDocumentsWithContext(ctx, &msdk.DocumentsQuery{
			Offset: offset,
			Limit:  documentIDListBatchSize,
			Fields: documentIDFields,
		}, &result)
		if err != nil {
			return nil, fmt.Errorf("get document IDs from %s (offset %d): %w", indexName, offset, err)
		}

		for _, hit := range result.Results {
			raw, ok := hit["id"]
			if !ok {
				continue
			}

			var id string
			if err := json.Unmarshal(raw, &id); err != nil {
				continue
			}

			allIDs = append(allIDs, id)
		}

		offset += documentIDListBatchSize
		if offset >= result.Total {
			break
		}
	}

	c.logger.Debug("retrieved all document IDs",
		slog.String("index", indexName),
		slog.Int("count", len(allIDs)))

	return allIDs, nil
}

// Search runs a full-text query against the specified index and returns raw
// JSON hits so callers can deserialize into their own document types.
func (c *Client) Search(ctx context.Context, req *SearchRequest) (*SearchResult, error) {
	sdkReq := &msdk.SearchRequest{
		Sort:   req.Sort,
		Offset: req.Offset,
		Limit:  req.Limit,
	}
	if req.Filter != "" {
		sdkReq.Filter = req.Filter
	}

	resp, err := c.sdk.Index(req.IndexName).SearchWithContext(ctx, req.Query, sdkReq)
	if err != nil {
		return nil, fmt.Errorf("search index %s: %w", req.IndexName, err)
	}

	hits := make([]json.RawMessage, 0, len(resp.Hits))
	for _, hit := range resp.Hits {
		raw, err := json.Marshal(hit)
		if err != nil {
			return nil, fmt.Errorf("marshal search hit: %w", err)
		}
		hits = append(hits, raw)
	}

	c.logger.Debug("search completed",
		slog.String("index", req.IndexName),
		slog.String("query", req.Query),
		slog.Int64("total_hits", resp.EstimatedTotalHits),
		slog.Int("hits", len(hits)))

	return &SearchResult{
		Hits:      hits,
		TotalHits: resp.EstimatedTotalHits,
	}, nil
}
