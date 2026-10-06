// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/secrets"
	"github.com/altessa-s/go-atlas/security/secrets/providers/vault"

	vaultApi "github.com/hashicorp/vault/api"
)

// An empty KV path answers LIST with an empty 404; the store must report an
// empty listing, not an error, so the Manager can drop the last secret.
func TestStorage_ListEmptyPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[]}`))
	}))
	t.Cleanup(srv.Close)

	client, err := vaultApi.NewClient(&vaultApi.Config{Address: srv.URL})
	require.NoError(t, err)
	client.SetToken("test")
	store, err := vault.New[string](client)
	require.NoError(t, err)

	list, err := store.List(t.Context())
	require.NoError(t, err)
	require.Empty(t, list)
}

// A key whose latest KV v2 version was deleted keeps its metadata, so it is
// still listed, but Get returns no data: the store must report it absent and
// leave it out of the listing, so the Manager can evict it.
func TestStorage_SoftDeletedLatestVersion(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "LIST" || r.URL.Query().Get("list") == "true" {
			_, _ = w.Write([]byte(`{"data":{"keys":["a2V5"]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"data":null,"metadata":{"version":2,"deletion_time":"2026-10-06T00:00:00Z","destroyed":false}}}`))
	}))
	t.Cleanup(srv.Close)

	client, err := vaultApi.NewClient(&vaultApi.Config{Address: srv.URL})
	require.NoError(t, err)
	client.SetToken("test")
	store, err := vault.New[string](client)
	require.NoError(t, err)

	list, err := store.List(t.Context())
	require.NoError(t, err)
	require.Empty(t, list)

	_, err = store.Value(t.Context(), "key")
	require.ErrorIs(t, err, secrets.ErrNotFound)

	for _, err := range store.Values(t.Context()) {
		require.NoError(t, err)
		t.Fatal("a deleted version must not be yielded")
	}
}

// A version scheduled for automatic deletion (delete_version_after) carries a
// future deletion_time but is still live: it must be read and listed.
func TestStorage_ScheduledDeletionStillLive(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "LIST" || r.URL.Query().Get("list") == "true" {
			_, _ = w.Write([]byte(`{"data":{"keys":["a2V5"]}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"data":{"data":"\"v\""},"metadata":{"version":3,"deletion_time":"2099-01-01T00:00:00Z","destroyed":false}}}`))
	}))
	t.Cleanup(srv.Close)

	client, err := vaultApi.NewClient(&vaultApi.Config{Address: srv.URL})
	require.NoError(t, err)
	client.SetToken("test")
	store, err := vault.New[string](client)
	require.NoError(t, err)

	list, err := store.List(t.Context())
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "v", list[0].Value)

	got, err := store.Value(t.Context(), "key")
	require.NoError(t, err)
	require.Equal(t, "v", got.Value)
}
