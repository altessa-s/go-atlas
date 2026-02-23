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
	if err != nil || revoked {
		t.Fatalf("expected not revoked, got revoked=%v err=%v", revoked, err)
	}

	filter.data["token1"] = true
	revoked, err = storage.IsRevoked(ctx, "token1")
	if err != nil || !revoked {
		t.Fatalf("expected revoked, got revoked=%v err=%v", revoked, err)
	}
}

func TestFilterRevocationStorage_MarkRevoked(t *testing.T) {
	filter := newMockFilter()
	storage := NewFilterRevocationStorage(filter, nil)
	ctx := t.Context()

	if err := storage.MarkRevoked(ctx, "token2", time.Hour); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	revoked, _ := storage.IsRevoked(ctx, "token2")
	if !revoked {
		t.Fatal("expected revoked after MarkRevoked")
	}
}

func TestFilterRevocationStorage_NilFilter(t *testing.T) {
	storage := NewFilterRevocationStorage(nil, nil)
	ctx := t.Context()

	revoked, err := storage.IsRevoked(ctx, "token")
	if err != nil || revoked {
		t.Fatal("expected false, nil for nil filter")
	}

	if err := storage.MarkRevoked(ctx, "token", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := storage.Sync(ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFilterRevocationStorage_Sync_NotRebuildable(t *testing.T) {
	filter := newMockFilter()
	storage := NewFilterRevocationStorage(filter, &FileRevocationLoader{Path: "/dev/null"})
	ctx := t.Context()

	err := storage.Sync(ctx)
	if err == nil {
		t.Fatal("expected error for non-rebuildable filter")
	}
}

func TestFileRevocationLoader_StreamValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "revoked.txt")
	if err := os.WriteFile(path, []byte("token1\ntoken2\ntoken3\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := &FileRevocationLoader{Path: path}
	ctx := t.Context()

	var values []string
	for v, err := range loader.StreamValues(ctx) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		values = append(values, v)
	}
	if len(values) != 3 {
		t.Fatalf("expected 3 values, got %d", len(values))
	}
}

func TestFileRevocationLoader_Count(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "revoked.txt")
	if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	loader := &FileRevocationLoader{Path: path}
	count, err := loader.Count(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected 3, got %d", count)
	}
}

func TestFileRevocationLoader_MissingFile(t *testing.T) {
	loader := &FileRevocationLoader{Path: "/nonexistent/path"}

	for _, err := range loader.StreamValues(t.Context()) {
		if err == nil {
			t.Fatal("expected error for missing file")
		}
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
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		values = append(values, v)
	}
	if len(values) != 2 {
		t.Fatalf("expected 2 values, got %d", len(values))
	}
}

func TestURLRevocationLoader_Count(t *testing.T) {
	loader := &URLRevocationLoader{URL: "http://example.com"}
	count, err := loader.Count(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if count != -1 {
		t.Fatalf("expected -1, got %d", count)
	}
}

func TestURLRevocationLoader_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	loader := &URLRevocationLoader{URL: srv.URL, Client: srv.Client()}
	for _, err := range loader.StreamValues(t.Context()) {
		if err == nil {
			t.Fatal("expected error for bad status")
		}
		break
	}
}
