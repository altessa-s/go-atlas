// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package vault_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

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
