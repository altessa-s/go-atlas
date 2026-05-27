// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"context"
	"errors"
	"log/slog"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	msdk "github.com/meilisearch/meilisearch-go"
)

// IndexSettings holds the attribute lists Meilisearch exposes for per-index
// full-text, filter, and sort behavior.
type IndexSettings struct {
	// SearchableAttributes are fields used for full-text matching.
	SearchableAttributes []string

	// FilterableAttributes are fields usable in filter expressions.
	FilterableAttributes []string

	// SortableAttributes are fields usable in [SearchRequest.Sort].
	SortableAttributes []string
}

// IndexDefinition describes one index the caller wants to ensure exists.
type IndexDefinition struct {
	// Name is the index UID in Meilisearch.
	Name string

	// PrimaryKey is the document field used as primary key.
	PrimaryKey string

	// Settings, when non-nil, are applied via [Client.UpdateIndexSettings]
	// after the index exists. nil leaves settings untouched.
	Settings *IndexSettings
}

// IndexExists reports whether indexName is present on the server.
// Returns false (no error) if the server responds with "index_not_found".
// ctx bounds the underlying GetIndex round-trip — passing a cancelled
// context aborts the call immediately instead of waiting for the SDK's
// per-request timeout.
func (c *Client) IndexExists(ctx context.Context, indexName string) (bool, error) {
	_, err := c.sdk.GetIndexWithContext(ctx, indexName)
	if err != nil {
		if classified := classifySDKError(err); classified != nil && errors.Is(classified, ErrIndexNotFound) {
			return false, nil
		}
		return false, coreerrs.Wrapf(err, "get index %s", indexName)
	}

	return true, nil
}

// EnsureIndex creates name if missing and applies settings. Idempotent:
// re-running with the same definition is safe — an existing index
// surfaces ErrIndexAlreadyExists from the create call, which we treat
// as a successful "create or update" outcome and fall through to the
// settings update.
func (c *Client) EnsureIndex(ctx context.Context, name, primaryKey string, settings *IndexSettings) error {
	_, err := c.sdk.CreateIndexWithContext(ctx, &msdk.IndexConfig{
		Uid:        name,
		PrimaryKey: primaryKey,
	})
	if err != nil {
		classified := classifySDKError(err)
		if classified == nil || !errors.Is(classified, ErrIndexAlreadyExists) {
			return coreerrs.Wrapf(err, "create index %s", name)
		}
		c.logger.DebugContext(ctx, "index already exists", slog.String("index", name))
	}

	if settings != nil {
		if err := c.UpdateIndexSettings(ctx, name, settings); err != nil {
			return err
		}
	}

	c.logger.InfoContext(ctx, "index ensured",
		slog.String("index", name),
		slog.String("primary_key", primaryKey))

	return nil
}

// UpdateIndexSettings applies the searchable, filterable, and sortable
// attribute lists to an existing index.
func (c *Client) UpdateIndexSettings(ctx context.Context, indexName string, settings *IndexSettings) error {
	_, err := c.sdk.Index(indexName).UpdateSettingsWithContext(ctx, &msdk.Settings{
		SearchableAttributes: settings.SearchableAttributes,
		FilterableAttributes: settings.FilterableAttributes,
		SortableAttributes:   settings.SortableAttributes,
	})
	if err != nil {
		return coreerrs.Wrapf(err, "update settings for %s", indexName)
	}

	return nil
}

// SetupIndexes ensures every index in defs exists with the configured
// settings. Call this once at startup; order is not significant and
// each definition is processed independently.
//
// Relies on [Client.EnsureIndex]'s built-in idempotency: an existing
// index surfaces ErrIndexAlreadyExists from the create call, which
// EnsureIndex silently absorbs and falls through to the settings
// update. The previous explicit IndexExists probe introduced a
// TOCTOU window where a concurrent process could create the index
// between the check and the create — relying on the existing-index
// error path eliminates that window.
//
// Uses the Client's own logger; SetupIndexes intentionally does not
// accept a logger parameter to avoid the "two loggers, which wins?"
// trap on per-call observability.
func (c *Client) SetupIndexes(ctx context.Context, defs []IndexDefinition) error {
	for _, def := range defs {
		if err := c.EnsureIndex(ctx, def.Name, def.PrimaryKey, def.Settings); err != nil {
			return coreerrs.Wrapf(err, "ensure index %s", def.Name)
		}
	}

	c.logger.InfoContext(ctx, "all indexes configured", slog.Int("count", len(defs)))

	return nil
}
