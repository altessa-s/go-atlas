// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package meilisearch provides a thin wrapper around the meilisearch-go SDK
// for full-text search and document indexing. It owns the HTTP client (so
// connections can be closed at shutdown or after a failed startup probe),
// requires an available health status at startup,
// and implements [IndexManager], [DocumentClient], [Searcher], [TaskWaiter],
// and [HealthChecker].
//
// # Features
//
//   - Owned HTTP client with configurable timeout and idle-connection cleanup.
//   - Same-origin redirects limited to ten hops and a 64 KiB health response limit.
//   - Index lifecycle helpers: IndexExists, EnsureIndex, UpdateIndexSettings, SetupIndexes.
//   - Search returning raw JSON hits so callers deserialize into their own document types.
//   - Pluggable construction via options or the factory/infrastructure/meilisearch builder.
//
// # Usage
//
//	c, err := meilisearch.New(ctx, "http://localhost:7700",
//	    meilisearch.WithAPIKey("masterKey"),
//	    meilisearch.WithLogger(logger),
//	)
//	if err != nil {
//	    return err
//	}
//	defer c.Close()
//
//	if err := c.SetupIndexes(ctx, defs); err != nil {
//	    return err
//	}
//
// Sanitized request and task errors unwrap to package sentinels such as
// [ErrRequestFailed] and [ErrTaskFailed].
package meilisearch
