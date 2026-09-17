// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
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
	storage := NewFilterRevocationStorage(filter, nil, nil)

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
	storage := NewFilterRevocationStorage(filter, nil, nil)
	ctx := t.Context()

	require.NoError(t, storage.MarkRevoked(ctx, "token2", time.Hour))

	revoked, _ := storage.IsRevoked(ctx, "token2")
	require.True(t, revoked, "expected revoked after MarkRevoked")
}

func TestFilterRevocationStorage_NilFilter(t *testing.T) {
	storage := NewFilterRevocationStorage(nil, nil, nil)
	ctx := t.Context()

	revoked, err := storage.IsRevoked(ctx, "token")
	require.NoError(t, err)
	require.False(t, revoked)

	require.NoError(t, storage.MarkRevoked(ctx, "token", 0))
	require.NoError(t, storage.Sync(ctx))
}

func TestFilterRevocationStorage_Sync_NotRebuildable(t *testing.T) {
	filter := newMockFilter()
	storage := NewFilterRevocationStorage(filter, &FileRevocationLoader{Path: "/dev/null"}, nil)
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

// errFilter is a Filter whose membership probe always fails, used to assert
// that a filter error propagates instead of being read as a revocation.
type errFilter struct{ err error }

func (f *errFilter) MightExist(context.Context, string) (bool, error) { return false, f.err }
func (f *errFilter) Add(context.Context, string) error                { return nil }
func (f *errFilter) AddBatch(context.Context, iter.Seq[string]) error { return nil }

// mockAuthoritative is an exact revocation store that records whether it was
// consulted, so tests can assert the filter short-circuits definite misses.
type mockAuthoritative struct {
	revoked map[string]bool
	err     error
	calls   int
}

func (a *mockAuthoritative) IsRevoked(_ context.Context, item string) (bool, error) {
	a.calls++
	if a.err != nil {
		return false, a.err
	}
	return a.revoked[item], nil
}

// A filter hit that the authoritative store does not confirm must not revoke the
// item. This is the false-positive path: without confirmation a 1% Bloom false
// positive rejects a valid token.
func TestFilterRevocationStorage_IsRevoked_FilterHitNotConfirmed(t *testing.T) {
	t.Parallel()

	filter := newMockFilter()
	filter.data["good-token"] = true // false positive
	auth := &mockAuthoritative{revoked: map[string]bool{}}
	storage := NewFilterRevocationStorage(filter, nil, auth)

	revoked, err := storage.IsRevoked(t.Context(), "good-token")
	require.NoError(t, err)
	require.False(t, revoked, "unconfirmed filter hit must not revoke")
	require.Equal(t, 1, auth.calls, "authoritative store must be consulted on a hit")
}

func TestFilterRevocationStorage_IsRevoked_FilterHitConfirmed(t *testing.T) {
	t.Parallel()

	filter := newMockFilter()
	filter.data["bad-token"] = true
	auth := &mockAuthoritative{revoked: map[string]bool{"bad-token": true}}
	storage := NewFilterRevocationStorage(filter, nil, auth)

	revoked, err := storage.IsRevoked(t.Context(), "bad-token")
	require.NoError(t, err)
	require.True(t, revoked)
	require.Equal(t, 1, auth.calls)
}

// A definite filter miss is exact, so the authoritative store is never touched.
func TestFilterRevocationStorage_IsRevoked_FilterMissSkipsAuthoritative(t *testing.T) {
	t.Parallel()

	auth := &mockAuthoritative{revoked: map[string]bool{"unseen": true}}
	storage := NewFilterRevocationStorage(newMockFilter(), nil, auth)

	revoked, err := storage.IsRevoked(t.Context(), "unseen")
	require.NoError(t, err)
	require.False(t, revoked)
	require.Zero(t, auth.calls, "a definite miss must not reach the authoritative store")
}

// Without an authoritative store an unconfirmed hit stays a revocation: the
// security invariant holds, at the cost of the false-positive rate.
func TestFilterRevocationStorage_IsRevoked_NoAuthoritativeKeepsHit(t *testing.T) {
	t.Parallel()

	filter := newMockFilter()
	filter.data["token"] = true
	storage := NewFilterRevocationStorage(filter, nil, nil)

	revoked, err := storage.IsRevoked(t.Context(), "token")
	require.NoError(t, err)
	require.True(t, revoked)
}

func TestFilterRevocationStorage_IsRevoked_FilterErrorPropagates(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("filter unavailable")
	auth := &mockAuthoritative{revoked: map[string]bool{}}
	storage := NewFilterRevocationStorage(&errFilter{err: sentinel}, nil, auth)

	revoked, err := storage.IsRevoked(t.Context(), "token")
	require.ErrorIs(t, err, sentinel)
	require.False(t, revoked)
	require.Zero(t, auth.calls)
}

func TestFilterRevocationStorage_IsRevoked_AuthoritativeErrorPropagates(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("store unavailable")
	filter := newMockFilter()
	filter.data["token"] = true
	storage := NewFilterRevocationStorage(filter, nil, &mockAuthoritative{err: sentinel})

	revoked, err := storage.IsRevoked(t.Context(), "token")
	require.ErrorIs(t, err, sentinel)
	require.False(t, revoked)
}
