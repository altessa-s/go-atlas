// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"context"
	"fmt"
	"log/slog"

	msdk "github.com/meilisearch/meilisearch-go"
)

// IndexSettings holds the attribute lists Meilisearch exposes for per-index
// full-text, filter, and sort behaviour.
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
func (c *Client) IndexExists(_ context.Context, indexName string) (bool, error) {
	_, err := c.sdk.GetIndex(indexName)
	if err != nil {
		if IsErrorIndexNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("get index %s: %w", indexName, err)
	}

	return true, nil
}

// EnsureIndex creates name if missing and applies settings. Idempotent:
// re-running with the same definition is safe.
func (c *Client) EnsureIndex(ctx context.Context, name, primaryKey string, settings *IndexSettings) error {
	_, err := c.sdk.CreateIndexWithContext(ctx, &msdk.IndexConfig{
		Uid:        name,
		PrimaryKey: primaryKey,
	})
	if err != nil {
		if !IsErrorIndexAlreadyExists(err) {
			return fmt.Errorf("create index %s: %w", name, err)
		}
		c.logger.Debug("index already exists", slog.String("index", name))
	}

	if settings != nil {
		if err := c.UpdateIndexSettings(ctx, name, settings); err != nil {
			return err
		}
	}

	c.logger.Info("index ensured",
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
		return fmt.Errorf("update settings for %s: %w", indexName, err)
	}

	return nil
}

// SetupIndexes creates missing indexes and updates settings on existing ones
// for every definition in defs. Call this once at startup; order does not
// matter and each definition is processed independently.
func SetupIndexes(ctx context.Context, c *Client, defs []IndexDefinition, logger *slog.Logger) error {
	for _, def := range defs {
		exists, err := c.IndexExists(ctx, def.Name)
		if err != nil {
			return fmt.Errorf("check index %s: %w", def.Name, err)
		}

		if exists {
			if def.Settings != nil {
				if err := c.UpdateIndexSettings(ctx, def.Name, def.Settings); err != nil {
					return err
				}
			}
			continue
		}

		if err := c.EnsureIndex(ctx, def.Name, def.PrimaryKey, def.Settings); err != nil {
			return fmt.Errorf("ensure index %s: %w", def.Name, err)
		}
		logger.Info("created index", slog.String("index", def.Name))
	}

	logger.Info("all indexes successfully configured")

	return nil
}
