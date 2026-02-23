// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"iter"
	"net/http"
	"os"
	"time"

	"github.com/altessa-s/go-atlas/data/probfilter"
)

// RevocationStorage defines a unified interface for checking and managing revoked items
// (tokens, jtis, or kids). It abstracts the underlying storage (e.g., Bloom filter, Redis).
type RevocationStorage interface {
	// IsRevoked checks if the given item (token, jti, or kid) is revoked.
	IsRevoked(ctx context.Context, item string) (bool, error)

	// MarkRevoked marks an item as revoked with an optional TTL.
	// This is typically called after a successful remote introspection that returns 'active: false'.
	MarkRevoked(ctx context.Context, item string, ttl time.Duration) error

	// Sync performs a synchronization of revoked items from an external source.
	// This method is compatible with service/scheduler.TaskFunc.
	Sync(ctx context.Context) error
}

// filterRevocationStorage is a RevocationStorage implementation using probabilistic filters.
type filterRevocationStorage struct {
	filter probfilter.Filter
	loader DataLoader
}

// NewFilterRevocationStorage creates a new revocation storage using a probabilistic filter.
func NewFilterRevocationStorage(filter Filter, loader DataLoader) RevocationStorage {
	return &filterRevocationStorage{
		filter: filter,
		loader: loader,
	}
}

// IsRevoked implements RevocationStorage.
func (s *filterRevocationStorage) IsRevoked(ctx context.Context, item string) (bool, error) {
	if s.filter == nil {
		return false, nil
	}
	return s.filter.MightExist(ctx, item)
}

// MarkRevoked implements RevocationStorage.
func (s *filterRevocationStorage) MarkRevoked(ctx context.Context, item string, _ time.Duration) error {
	if s.filter == nil {
		return nil
	}
	return s.filter.Add(ctx, item)
}

// Sync implements RevocationStorage.
func (s *filterRevocationStorage) Sync(ctx context.Context) error {
	if s.filter == nil || s.loader == nil {
		return nil
	}

	rebuildable, ok := s.filter.(probfilter.RebuildableFilter)
	if !ok {
		return fmt.Errorf("filter type %T does not support rebuilding/syncing", s.filter)
	}

	return rebuildable.Rebuild(ctx, s.loader)
}

// FileRevocationLoader loads revoked items from a local file (one per line).
type FileRevocationLoader struct {
	Path string
}

// StreamValues implements DataLoader.
func (l *FileRevocationLoader) StreamValues(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		file, err := os.Open(l.Path)
		if err != nil {
			yield("", err)
			return
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				yield("", ctx.Err())
				return
			default:
				if !yield(scanner.Text(), nil) {
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			yield("", err)
		}
	}
}

// Count implements DataLoader.
func (l *FileRevocationLoader) Count(_ context.Context) (int64, error) {
	// Simple count by reading file
	file, err := os.Open(l.Path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	var count int64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

// URLRevocationLoader loads revoked items from a remote URL (one per line).
type URLRevocationLoader struct {
	URL    string
	Client *http.Client
}

// StreamValues implements DataLoader.
func (l *URLRevocationLoader) StreamValues(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.URL, nil)
		if err != nil {
			yield("", err)
			return
		}

		client := l.Client
		if client == nil {
			client = http.DefaultClient
		}

		resp, err := client.Do(req)
		if err != nil {
			yield("", err)
			return
		}
		defer func() {
			// Drain body to allow connection reuse
			_, _ = io.Copy(io.Discard, resp.Body) //nolint:errcheck // best-effort drain for connection reuse
			_ = resp.Body.Close()
		}()

		if resp.StatusCode != http.StatusOK {
			yield("", fmt.Errorf("URL revocation loader: unexpected status %d", resp.StatusCode))
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			// Check context cancellation before processing each line
			select {
			case <-ctx.Done():
				yield("", ctx.Err())
				return
			default:
			}
			if !yield(scanner.Text(), nil) {
				return
			}
		}

		if err := scanner.Err(); err != nil {
			yield("", err)
		}
	}
}

// Count implements DataLoader.
func (l *URLRevocationLoader) Count(_ context.Context) (int64, error) {
	return -1, nil // Unknown count for URLs
}
