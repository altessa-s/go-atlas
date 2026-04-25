// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// mockFilter implements probfilter.Filter for testing.
type mockFilter struct {
	data map[string]bool
}

func newMockFilter() *mockFilter {
	return &mockFilter{data: make(map[string]bool)}
}

func (f *mockFilter) MightExist(_ context.Context, value string) (bool, error) {
	return f.data[value], nil
}

func (f *mockFilter) Add(_ context.Context, value string) error {
	f.data[value] = true
	return nil
}

func (f *mockFilter) AddBatch(_ context.Context, values iter.Seq[string]) error {
	for v := range values {
		f.data[v] = true
	}
	return nil
}

func TestFilterRevocationStorage_IsRevoked(t *testing.T) {
	filter := newMockFilter()
	storage := NewFilterRevocationStorage(filter, nil)

	ctx := t.Context()

	revoked, err := storage.IsRevoked(ctx, "token1")
	require.NoError(t, err)
	require.False(t, revoked)

	filter.data["token1"] = true
	revoked, err = storage.IsRevoked(ctx, "token1")
	require.NoError(t, err)
	require.True(t, revoked)
}

func TestFilterRevocationStorage_MarkRevoked(t *testing.T) {
	filter := newMockFilter()
	storage := NewFilterRevocationStorage(filter, nil)
	ctx := t.Context()

	require.NoError(t, storage.MarkRevoked(ctx, "token2", time.Hour))

	revoked, _ := storage.IsRevoked(ctx, "token2")
	require.True(t, revoked, "expected revoked after MarkRevoked")
}

func TestFilterRevocationStorage_NilFilter(t *testing.T) {
	storage := NewFilterRevocationStorage(nil, nil)
	ctx := t.Context()

	revoked, err := storage.IsRevoked(ctx, "token")
	require.NoError(t, err)
	require.False(t, revoked)

	require.NoError(t, storage.MarkRevoked(ctx, "token", 0))
	require.NoError(t, storage.Sync(ctx))
}

func TestFilterRevocationStorage_Sync_NotRebuildable(t *testing.T) {
	filter := newMockFilter()
	storage := NewFilterRevocationStorage(filter, &FileRevocationLoader{Path: "/dev/null"})
	ctx := t.Context()

	err := storage.Sync(ctx)
	require.Error(t, err, "expected error for non-rebuildable filter")
}

func TestFileRevocationLoader_StreamValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "revoked.txt")
	require.NoError(t, os.WriteFile(path, []byte("token1\ntoken2\ntoken3\n"), 0o644))

	loader := &FileRevocationLoader{Path: path}
	ctx := t.Context()

	var values []string
	for v, err := range loader.StreamValues(ctx) {
		require.NoError(t, err)
		values = append(values, v)
	}
	require.Len(t, values, 3)
}

func TestFileRevocationLoader_Count(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "revoked.txt")
	require.NoError(t, os.WriteFile(path, []byte("a\nb\nc\n"), 0o644))

	loader := &FileRevocationLoader{Path: path}
	count, err := loader.Count(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(3), count)
}

func TestFileRevocationLoader_MissingFile(t *testing.T) {
	loader := &FileRevocationLoader{Path: "/nonexistent/path"}

	for _, err := range loader.StreamValues(t.Context()) {
		require.Error(t, err, "expected error for missing file")
		break
	}
}

func TestURLRevocationLoader_StreamValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "revoked1")
		fmt.Fprintln(w, "revoked2")
	}))
	defer srv.Close()

	loader := &URLRevocationLoader{URL: srv.URL, Client: srv.Client()}

	var values []string
	for v, err := range loader.StreamValues(t.Context()) {
		require.NoError(t, err)
		values = append(values, v)
	}
	require.Len(t, values, 2)
}

func TestURLRevocationLoader_Count(t *testing.T) {
	loader := &URLRevocationLoader{URL: "http://example.com"}
	count, err := loader.Count(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(-1), count)
}

func TestURLRevocationLoader_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	loader := &URLRevocationLoader{URL: srv.URL, Client: srv.Client()}
	for _, err := range loader.StreamValues(t.Context()) {
		require.Error(t, err, "expected error for bad status")
		break
	}
}
