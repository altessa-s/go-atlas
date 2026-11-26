// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/store/meilisearch"

	msdk "github.com/meilisearch/meilisearch-go"
)

func newMeiliHTTPServer(t *testing.T, handler http.HandlerFunc) *meilisearch.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, available)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	client, err := meilisearch.New(t.Context(), srv.URL)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

func TestGetAllDocumentIDs_NumericAndStringIDs(t *testing.T) {
	t.Parallel()

	client := newMeiliHTTPServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[{"id":42},{"id":"abc"}],"total":2}`)
	})

	ids, err := client.GetAllDocumentIDs(t.Context(), "items")
	require.NoError(t, err)
	require.Equal(t, []string{"42", "abc"}, ids)
}

func TestGetAllDocumentIDs_RejectsMissingOrUndecodablePrimaryKey(t *testing.T) {
	t.Parallel()

	const marker = "secret-doc-marker-9911"
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "missing key", body: `{"results":[{"other":"` + marker + `"}],"total":1}`},
		{name: "null key", body: `{"results":[{"id":null}],"total":1}`},
		{name: "object key", body: `{"results":[{"id":{"secret":"` + marker + `"}}],"total":1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := newMeiliHTTPServer(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.body)
			})
			ids, err := client.GetAllDocumentIDs(t.Context(), "items")
			require.Error(t, err)
			require.Nil(t, ids)
			require.NotContains(t, err.Error(), marker)
		})
	}
}

func TestClient_OmitsServerMessageFromTaskAndSearchErrors(t *testing.T) {
	t.Parallel()

	const marker = "secret-pii-marker-98765"

	client := newMeiliHTTPServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/tasks/9":
			_, _ = io.WriteString(w, `{"uid":9,"status":"failed","error":{"message":"Document `+marker+` is invalid","code":"invalid_document_id","type":"invalid_request"}}`)
		case "/indexes/items/search":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"message":"Filter `+marker+` is invalid","code":"invalid_search_filter","type":"invalid_request"}`)
		default:
			http.NotFound(w, r)
		}
	})

	t.Run("failed task", func(t *testing.T) {
		t.Parallel()
		taskErr := client.WaitForTask(t.Context(), 9, time.Millisecond)
		require.ErrorIs(t, taskErr, meilisearch.ErrTaskFailed)
		require.Equal(t, meilisearch.ErrTaskFailed, errors.Unwrap(taskErr))
		require.Contains(t, taskErr.Error(), "invalid_document_id")
		require.NotContains(t, taskErr.Error(), marker)
	})

	t.Run("rejected search", func(t *testing.T) {
		t.Parallel()
		_, searchErr := client.Search(t.Context(), &meilisearch.SearchRequest{
			IndexName: "items",
			Query:     marker,
			Filter:    "email = '" + marker + "'",
		})
		require.Error(t, searchErr)
		cause := errors.Unwrap(searchErr)
		require.Equal(t, meilisearch.ErrRequestFailed, cause)
		require.Nil(t, errors.Unwrap(cause))
		require.NotContains(t, cause.Error(), marker)
		var sdkErr *msdk.Error
		require.False(t, errors.As(searchErr, &sdkErr))
		require.Contains(t, searchErr.Error(), "invalid_search_filter")
		require.NotContains(t, searchErr.Error(), marker)
	})
}
