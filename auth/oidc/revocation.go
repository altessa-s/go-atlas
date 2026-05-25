// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"bufio"
	"context"
	"io"
	"iter"
	"net/http"
	"os"
	"time"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
	httpclient "github.com/altessa-s/go-atlas/transport/http/client"
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
	filter Filter
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

	rebuildable, ok := s.filter.(RebuildableFilter)
	if !ok {
		return coreerrs.Wrapf(ErrFilterNotRebuildable, "filter type %T", s.filter)
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
		defer func() { _ = file.Close() }()

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
	defer func() { _ = file.Close() }()

	var count int64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}

// URLRevocationLoader loads revoked items from a remote URL (one per line).
//
// Client is optional when the loader is wired into an OIDC [Provider] —
// [NewProvider] injects its shared HTTP client via [SetHTTPClient] (the
// loader implements [httpclient.HTTPClientSetter]) so the revocation
// refresh reuses the same pool, retry policy and proxy resolver as
// discovery/JWKS/userinfo.
//
// Setting Client explicitly is still supported as an escape hatch for
// tests or callers using URLRevocationLoader outside of [Provider]. In
// that case StreamValues fails fast if Client is nil rather than
// silently falling back to http.DefaultClient — match the failure with
// [ErrLoaderClientNotConfigured] via [errors.Is].
type URLRevocationLoader struct {
	URL    string
	Client *http.Client
}

// Compile-time guarantee that URLRevocationLoader satisfies the
// HTTPClientSetter contract Provider relies on for shared-client
// injection. Renaming or removing SetHTTPClient breaks at build time
// instead of silently disabling the wiring at runtime.
var _ httpclient.HTTPClientSetter = (*URLRevocationLoader)(nil)

// SetHTTPClient implements [httpclient.HTTPClientSetter] for URLRevocationLoader.
// It adopts the supplied client only when no Client was set explicitly,
// preserving the caller's escape-hatch override.
func (l *URLRevocationLoader) SetHTTPClient(c *http.Client) {
	if l.Client == nil {
		l.Client = c
	}
}

// StreamValues implements DataLoader.
func (l *URLRevocationLoader) StreamValues(ctx context.Context) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		if l.Client == nil {
			yield("", coreerrs.Wrap(ErrLoaderClientNotConfigured, "set the Client field or wire the loader via oidc.NewProvider"))
			return
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.URL, nil)
		if err != nil {
			yield("", err)
			return
		}

		resp, err := l.Client.Do(req)
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
			yield("", coreerrs.Wrapf(ErrRevocationLoadFailed, "URL revocation loader: status %d", resp.StatusCode))
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
