// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"cmp"
	"context"
	"encoding/json"
	"log/slog"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	msdk "github.com/meilisearch/meilisearch-go"
)

// documentIDListBatchSize is the page size [Client.GetAllDocumentIDs] uses
// when paginating through an index.
const documentIDListBatchSize int64 = 1000

// defaultPrimaryKeyField is the field name used by [Client.GetAllDocumentIDs]
// when the caller does not pass an explicit primary key. Meilisearch's
// recommended default is "id", but any index can use a different key —
// callers using a non-default primary key should pass it explicitly via
// [Client.GetAllDocumentIDsWithPrimaryKey] instead.
const defaultPrimaryKeyField = "id"

// IndexDocuments adds or updates documents in indexName. documents must be a
// slice (or array) of structs/maps that include the index's primary key.
// Returns the Meilisearch task UID for tracking the async operation.
func (c *Client) IndexDocuments(ctx context.Context, indexName string, documents any) (int64, error) {
	task, err := c.sdk.Index(indexName).AddDocumentsWithContext(ctx, documents, nil)
	if err != nil {
		return 0, coreerrs.Wrapf(err, "index documents in %s", indexName)
	}

	c.logger.DebugContext(ctx, "documents indexed",
		slog.String("index", indexName),
		slog.Int64("task_uid", task.TaskUID))

	return task.TaskUID, nil
}

// DeleteDocument removes a single document by ID. Returns task UID.
func (c *Client) DeleteDocument(ctx context.Context, indexName, documentID string) (int64, error) {
	task, err := c.sdk.Index(indexName).DeleteDocumentWithContext(ctx, documentID, nil)
	if err != nil {
		return 0, coreerrs.Wrapf(err, "delete document %s from %s", documentID, indexName)
	}

	c.logger.DebugContext(ctx, "document deleted",
		slog.String("index", indexName),
		slog.String("document_id", documentID),
		slog.Int64("task_uid", task.TaskUID))

	return task.TaskUID, nil
}

// DeleteDocuments removes multiple documents by ID. Returns task UID.
func (c *Client) DeleteDocuments(ctx context.Context, indexName string, documentIDs []string) (int64, error) {
	task, err := c.sdk.Index(indexName).DeleteDocumentsWithContext(ctx, documentIDs, nil)
	if err != nil {
		return 0, coreerrs.Wrapf(err, "delete documents from %s", indexName)
	}

	c.logger.DebugContext(ctx, "documents deleted",
		slog.String("index", indexName),
		slog.Int("count", len(documentIDs)),
		slog.Int64("task_uid", task.TaskUID))

	return task.TaskUID, nil
}

// DeleteDocumentsByFilter removes documents matching the given Meilisearch
// filter expression. Returns task UID.
//
// SECURITY: filter is passed unchanged to Meilisearch. The Meilisearch
// filter DSL is not SQL, but it still supports field comparisons and
// expression composition (AND / OR / NOT / IN). Building filter from
// concatenated user input lets an attacker widen the deletion to
// documents they should not be able to touch (e.g. "tenant_id =
// 'their_id' OR true" → wipes the whole index). Construct filter via a
// trusted DSL builder, escape user-supplied values, or restrict the
// caller's role at the Meilisearch API-key level.
func (c *Client) DeleteDocumentsByFilter(ctx context.Context, indexName, filter string) (int64, error) {
	task, err := c.sdk.Index(indexName).DeleteDocumentsByFilterWithContext(ctx, filter, nil)
	if err != nil {
		return 0, coreerrs.Wrapf(err, "delete documents by filter from %s", indexName)
	}

	// Filter expressions can encode caller-supplied values; log only at
	// Debug level and never elevate to Info without redaction.
	c.logger.DebugContext(ctx, "documents deleted by filter",
		slog.String("index", indexName),
		slog.String("filter", filter),
		slog.Int64("task_uid", task.TaskUID))

	return task.TaskUID, nil
}

// FetchDocuments returns a page of raw JSON documents from indexName that
// match the optional Meilisearch filter expression. offset and limit
// paginate the result set; the returned [FetchResult.Total] is the total
// number of matching documents across all pages — use it to decide whether
// another page is worth fetching. Pass filter="" to fetch unfiltered
// documents.
//
// SECURITY: filter is passed unchanged to Meilisearch. Construct it via a
// trusted DSL builder or escape user-supplied values — see
// [Client.DeleteDocumentsByFilter] for the same caveat.
//
// The filter is NOT logged — it can carry caller-supplied values that
// would route PII through the application log. This matches
// [Client.Search]'s policy and intentionally diverges from
// [Client.DeleteDocumentsByFilter].
func (c *Client) FetchDocuments(ctx context.Context, indexName, filter string, offset, limit int64) (*FetchResult, error) {
	query := &msdk.DocumentsQuery{Offset: offset, Limit: limit}
	if filter != "" {
		query.Filter = filter
	}

	var result msdk.DocumentsResult
	if err := c.sdk.Index(indexName).GetDocumentsWithContext(ctx, query, &result); err != nil {
		return nil, coreerrs.Wrapf(err, "fetch documents from %s", indexName)
	}

	hits := make([]json.RawMessage, 0, len(result.Results))
	for _, hit := range result.Results {
		raw, err := json.Marshal(hit)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "marshal fetched hit")
		}
		hits = append(hits, raw)
	}

	c.logger.DebugContext(ctx, "documents fetched",
		slog.String("index", indexName),
		slog.Int64("total", result.Total),
		slog.Int("count", len(hits)))

	return &FetchResult{Hits: hits, Total: result.Total}, nil
}

// GetAllDocumentIDs paginates through indexName and returns every document's
// primary key. Assumes the index's primary key is the default "id" field —
// callers using a different primary key must use
// [Client.GetAllDocumentIDsWithPrimaryKey] instead.
func (c *Client) GetAllDocumentIDs(ctx context.Context, indexName string) ([]string, error) {
	return c.GetAllDocumentIDsWithPrimaryKey(ctx, indexName, defaultPrimaryKeyField)
}

// GetAllDocumentIDsWithPrimaryKey paginates through indexName and returns
// every document's primary-key value. primaryKey names the field to
// project — pass "" to fall back to [defaultPrimaryKeyField]. Pulls
// [documentIDListBatchSize] IDs per round-trip and reads only the named
// field, so the body of each document is never transferred.
//
// Unmarshal failures on individual hits are logged at Warn (with the
// offending field name) and the hit is skipped; a misconfigured
// primary-key type (e.g. an int-typed key extracted as string) would
// otherwise return an empty slice with no diagnostic.
func (c *Client) GetAllDocumentIDsWithPrimaryKey(ctx context.Context, indexName, primaryKey string) ([]string, error) {
	primaryKey = cmp.Or(primaryKey, defaultPrimaryKeyField)
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
			Fields: []string{primaryKey},
		}, &result)
		if err != nil {
			return nil, coreerrs.Wrapf(err, "get document IDs from %s (offset %d)", indexName, offset)
		}

		for _, hit := range result.Results {
			raw, ok := hit[primaryKey]
			if !ok {
				c.logger.WarnContext(ctx, "primary key field missing from document",
					slog.String("index", indexName),
					slog.String("primary_key", primaryKey))
				continue
			}

			var id string
			if err := json.Unmarshal(raw, &id); err != nil {
				c.logger.WarnContext(ctx, "failed to decode primary key as string — check primary-key type",
					slog.String("index", indexName),
					slog.String("primary_key", primaryKey),
					slog.Any("error", err))
				continue
			}

			allIDs = append(allIDs, id)
		}

		offset += documentIDListBatchSize
		if offset >= result.Total {
			break
		}
	}

	c.logger.DebugContext(ctx, "retrieved all document IDs",
		slog.String("index", indexName),
		slog.String("primary_key", primaryKey),
		slog.Int("count", len(allIDs)))

	return allIDs, nil
}

// Search runs a full-text query against the specified index and returns raw
// JSON hits so callers can deserialize into their own document types.
//
// Search queries can carry caller-supplied values (email addresses,
// names, free-text). The query string is NOT logged — operators that
// need it for debugging should enable Meilisearch-side request logging
// rather than route PII through the application log.
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
		return nil, coreerrs.Wrapf(err, "search index %s", req.IndexName)
	}

	hits := make([]json.RawMessage, 0, len(resp.Hits))
	for _, hit := range resp.Hits {
		raw, err := json.Marshal(hit)
		if err != nil {
			return nil, coreerrs.WrapOperation(err, "marshal search hit")
		}
		hits = append(hits, raw)
	}

	c.logger.DebugContext(ctx, "search completed",
		slog.String("index", req.IndexName),
		slog.Int64("estimated_total_hits", resp.EstimatedTotalHits),
		slog.Int("hits", len(hits)))

	return &SearchResult{
		Hits:               hits,
		EstimatedTotalHits: resp.EstimatedTotalHits,
	}, nil
}
