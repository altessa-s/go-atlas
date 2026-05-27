// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package meilisearch provides a thin wrapper around the meilisearch-go SDK
// for full-text search and document indexing. It owns the HTTP client (so
// connections can be closed at shutdown), performs a health check at startup,
// and exposes a small interface (MeilisearchClient) covering indexing,
// deletion, and search operations.
//
// # Features
//
//   - Owned HTTP client with configurable timeout and idle-connection cleanup.
//   - Index lifecycle helpers: IndexExists, EnsureIndex, UpdateIndexSettings, SetupIndexes.
//   - Search returning raw JSON hits so callers deserialize into their own document types.
//   - Pluggable construction via options or the infrastructure/meilisearch/factory builder.
//
// # Usage
//
//	c, err := meilisearch.New("http://localhost:7700",
//	    meilisearch.WithAPIKey("masterKey"),
//	    meilisearch.WithLogger(logger),
//	)
//	if err != nil {
//	    return err
//	}
//	defer c.Close()
//
//	if err := meilisearch.SetupIndexes(ctx, c, defs, logger); err != nil {
//	    return err
//	}
package meilisearch
