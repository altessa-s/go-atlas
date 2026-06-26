// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package meilisearch

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	msdk "github.com/meilisearch/meilisearch-go"
)

// newSDKErrorWithCode produces an *msdk.Error with the given API error
// code. The MeilisearchApiError field has an unexported type, so we
// can't construct it via struct literal — but the field itself is
// exported and its Code subfield is too, so field assignment works.
// Centralized here so every test that needs a fake SDK error uses the
// same shape.
func newSDKErrorWithCode(code string) *msdk.Error {
	e := &msdk.Error{}
	e.MeilisearchApiError.Code = code
	return e
}

// TestClient_Health_PassesContextThrough verifies the post-fix
// HealthWithContext invocation: the SDK call must receive the caller's
// ctx so a cancelled / deadline-bound context aborts the probe early.
func TestClient_Health_PassesContextThrough(t *testing.T) {
	t.Parallel()

	var captured context.Context
	sdk := &fakeSDK{
		healthFn: func(ctx context.Context) (*msdk.Health, error) {
			captured = ctx
			return &msdk.Health{Status: "available"}, nil
		},
	}
	c := newTestClient(sdk)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	require.NoError(t, c.Health(ctx))
	require.NotNil(t, captured, "Health must reach the SDK")
	require.Same(t, ctx, captured, "Health must pass the caller's ctx unchanged — the PR-43 review flagged a sdk.Health() call that dropped it")
}

// TestClient_Health_WrapsSDKError covers the coreerrs migration: every
// SDK error must round-trip through the wrap so structured operation
// labels reach the logs.
func TestClient_Health_WrapsSDKError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("network down")
	sdk := &fakeSDK{
		healthFn: func(_ context.Context) (*msdk.Health, error) {
			return nil, sentinel
		},
	}
	c := newTestClient(sdk)

	err := c.Health(t.Context())
	require.Error(t, err)
	require.ErrorIs(t, err, sentinel, "wrapped error must preserve the SDK error for errors.Is")
}

// TestClient_IndexExists_UsesContextVariant is the regression guard
// for the PR-43 IndexExists ctx-discard bug. Asserts GetIndexWithContext
// (the context-aware variant) is invoked, not GetIndex.
func TestClient_IndexExists_UsesContextVariant(t *testing.T) {
	t.Parallel()

	var captured context.Context
	sdk := &fakeSDK{
		getIndexFn: func(ctx context.Context, _ string) (*msdk.IndexResult, error) {
			captured = ctx
			return &msdk.IndexResult{UID: "things"}, nil
		},
	}
	c := newTestClient(sdk)

	ctx := t.Context()
	exists, err := c.IndexExists(ctx, "things")
	require.NoError(t, err)
	require.True(t, exists)
	require.Same(t, ctx, captured, "IndexExists must pass ctx to GetIndexWithContext")
}

// TestClient_IndexExists_TreatsIndexNotFoundAsAbsent confirms the
// classifySDKError + ErrIndexNotFound integration: a Meilisearch
// "index_not_found" response yields (false, nil), not an error.
func TestClient_IndexExists_TreatsIndexNotFoundAsAbsent(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDK{
		getIndexFn: func(_ context.Context, _ string) (*msdk.IndexResult, error) {
			return nil, newSDKErrorWithCode(errCodeIndexNotFound)
		},
	}
	c := newTestClient(sdk)

	exists, err := c.IndexExists(t.Context(), "missing")
	require.NoError(t, err, "index_not_found must surface as (false, nil), not an error")
	require.False(t, exists)
}

// TestErrIndexNotFound_ErrorsIsViaPredicate exercises the sentinel
// errors that the PR-43 review asked for. errors.Is(err, ErrIndexNotFound)
// must match a wrapped SDK error with the "index_not_found" code.
func TestErrIndexNotFound_ErrorsIsViaPredicate(t *testing.T) {
	t.Parallel()

	sdkErr := newSDKErrorWithCode(errCodeIndexNotFound)
	classified := classifySDKError(sdkErr)
	require.NotNil(t, classified, "classifySDKError must recognize index_not_found")
	require.ErrorIs(t, classified, ErrIndexNotFound, "errors.Is must match the sentinel")
	require.ErrorIs(t, classified, sdkErr, "classified error must still wrap the original SDK error")
}

// TestErrIndexAlreadyExists_ErrorsIsViaPredicate is the symmetric case.
func TestErrIndexAlreadyExists_ErrorsIsViaPredicate(t *testing.T) {
	t.Parallel()

	sdkErr := newSDKErrorWithCode(errCodeIndexAlreadyExists)
	classified := classifySDKError(sdkErr)
	require.NotNil(t, classified)
	require.ErrorIs(t, classified, ErrIndexAlreadyExists)
}

// TestClassifySDKError_UnknownCodeReturnsNil pins the negative path:
// an SDK error with an unrecognized code is not classified, so the
// caller falls through to the generic Wrapf path.
func TestClassifySDKError_UnknownCodeReturnsNil(t *testing.T) {
	t.Parallel()

	sdkErr := newSDKErrorWithCode("some_other_code")
	require.Nil(t, classifySDKError(sdkErr))
	require.Nil(t, classifySDKError(nil))
	require.Nil(t, classifySDKError(errors.New("plain error")))
}

// TestClient_EnsureIndex_IdempotentOnAlreadyExists pins the
// SetupIndexes-relies-on-this contract: an existing index surfaces
// from CreateIndex as ErrIndexAlreadyExists and EnsureIndex absorbs
// it, falling through to UpdateSettings when settings are provided.
// Without this guarantee the simplified SetupIndexes (no upfront
// IndexExists probe) would fail on the second run.
func TestClient_EnsureIndex_IdempotentOnAlreadyExists(t *testing.T) {
	t.Parallel()

	var settingsCalled bool
	idx := &fakeIndex{
		updateSettingsFn: func(_ context.Context, _ *msdk.Settings) (*msdk.TaskInfo, error) {
			settingsCalled = true
			return &msdk.TaskInfo{TaskUID: 2}, nil
		},
	}
	sdk := &fakeSDK{
		createIndexFn: func(_ context.Context, _ *msdk.IndexConfig) (*msdk.TaskInfo, error) {
			return nil, newSDKErrorWithCode(errCodeIndexAlreadyExists)
		},
		indexFn: func(_ string) msdk.IndexManager { return idx },
	}
	c := newTestClient(sdk)

	settings := &IndexSettings{SearchableAttributes: []string{"name"}}
	require.NoError(t, c.EnsureIndex(t.Context(), "things", "id", settings))
	require.True(t, settingsCalled, "EnsureIndex must apply settings even when the create call returned 'already exists'")
}

// TestClient_SetupIndexes_RunsEnsureForEveryDef is the post-fix
// contract: SetupIndexes is now a Client method (no duplicate logger)
// that delegates to EnsureIndex for every entry. Asserting that each
// def's CreateIndex is invoked proves the new method threads through
// every definition without the TOCTOU-prone IndexExists probe.
func TestClient_SetupIndexes_RunsEnsureForEveryDef(t *testing.T) {
	t.Parallel()

	var created []string
	sdk := &fakeSDK{
		createIndexFn: func(_ context.Context, cfg *msdk.IndexConfig) (*msdk.TaskInfo, error) {
			created = append(created, cfg.Uid)
			return &msdk.TaskInfo{TaskUID: 1}, nil
		},
	}
	c := newTestClient(sdk)

	defs := []IndexDefinition{
		{Name: "alpha", PrimaryKey: "id"},
		{Name: "beta", PrimaryKey: "uid"},
	}
	require.NoError(t, c.SetupIndexes(t.Context(), defs))
	require.Equal(t, []string{"alpha", "beta"}, created)
}

// TestClient_DeleteIndex_PassesNameAndReturnsTaskUID is the happy-path
// coverage for the post-swap cleanup method: the index name must reach the
// context-aware SDK variant and the task UID must be propagated so callers
// can await it with WaitForTask.
func TestClient_DeleteIndex_PassesNameAndReturnsTaskUID(t *testing.T) {
	t.Parallel()

	const wantUID int64 = 17
	var gotName string
	sdk := &fakeSDK{
		deleteIndexFn: func(_ context.Context, uid string) (*msdk.TaskInfo, error) {
			gotName = uid
			return &msdk.TaskInfo{TaskUID: wantUID}, nil
		},
	}
	c := newTestClient(sdk)

	got, err := c.DeleteIndex(t.Context(), "workers_new")
	require.NoError(t, err)
	require.Equal(t, wantUID, got)
	require.Equal(t, "workers_new", gotName)
}

// TestClient_DeleteIndex_TreatsIndexNotFoundAsNoOp confirms idempotent
// cleanup: an already-absent index surfaces "index_not_found", which must
// collapse to a (0, nil) no-op so a re-run after a partial reindex is safe.
func TestClient_DeleteIndex_TreatsIndexNotFoundAsNoOp(t *testing.T) {
	t.Parallel()

	sdk := &fakeSDK{
		deleteIndexFn: func(_ context.Context, _ string) (*msdk.TaskInfo, error) {
			return nil, newSDKErrorWithCode(errCodeIndexNotFound)
		},
	}
	c := newTestClient(sdk)

	uid, err := c.DeleteIndex(t.Context(), "missing")
	require.NoError(t, err, "index_not_found must surface as a (0, nil) no-op, not an error")
	require.Zero(t, uid)
}

// TestClient_DeleteIndex_WrapsSDKError confirms an unclassified SDK error
// round-trips through coreerrs so errors.Is still reaches the original.
func TestClient_DeleteIndex_WrapsSDKError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("delete rejected")
	sdk := &fakeSDK{
		deleteIndexFn: func(_ context.Context, _ string) (*msdk.TaskInfo, error) {
			return nil, sentinel
		},
	}
	c := newTestClient(sdk)

	_, err := c.DeleteIndex(t.Context(), "workers_new")
	require.Error(t, err)
	require.ErrorIs(t, err, sentinel)
}

// TestClient_IndexDocuments_ReturnsTaskUID is the happy-path coverage
// for the most-called write method. Confirms ctx propagation and
// task UID extraction.
func TestClient_IndexDocuments_ReturnsTaskUID(t *testing.T) {
	t.Parallel()

	const wantUID int64 = 42
	idx := &fakeIndex{
		addDocumentsFn: func(_ context.Context, _ any, _ *msdk.DocumentOptions) (*msdk.TaskInfo, error) {
			return &msdk.TaskInfo{TaskUID: wantUID}, nil
		},
	}
	sdk := &fakeSDK{indexFn: func(_ string) msdk.IndexManager { return idx }}
	c := newTestClient(sdk)

	got, err := c.IndexDocuments(t.Context(), "things", []map[string]string{{"id": "1"}})
	require.NoError(t, err)
	require.Equal(t, wantUID, got)
}

// TestClient_Search_ReturnsRawHits guards the hit-marshaling loop:
// SDK hits come back as msdk.Hits (= []msdk.Hit, where Hit is
// map[string]json.RawMessage). We re-marshal each as json.RawMessage
// so callers can decode into their own struct types. The conversion
// must round-trip the underlying document.
func TestClient_Search_ReturnsRawHits(t *testing.T) {
	t.Parallel()

	idx := &fakeIndex{
		searchFn: func(_ context.Context, _ string, _ *msdk.SearchRequest) (*msdk.SearchResponse, error) {
			return &msdk.SearchResponse{
				Hits: msdk.Hits{
					{
						"id":    json.RawMessage(`"1"`),
						"title": json.RawMessage(`"alpha"`),
					},
				},
				EstimatedTotalHits: 7,
			}, nil
		},
	}
	sdk := &fakeSDK{indexFn: func(_ string) msdk.IndexManager { return idx }}
	c := newTestClient(sdk)

	result, err := c.Search(t.Context(), &SearchRequest{IndexName: "things", Query: "alpha"})
	require.NoError(t, err)
	require.Equal(t, int64(7), result.EstimatedTotalHits, "EstimatedTotalHits must round-trip the SDK field (PR-43 review: was misnamed TotalHits)")
	require.Len(t, result.Hits, 1)

	var hit map[string]string
	require.NoError(t, json.Unmarshal(result.Hits[0], &hit))
	require.Equal(t, "1", hit["id"])
	require.Equal(t, "alpha", hit["title"])
}

// TestClient_GetAllDocumentIDsWithPrimaryKey_UsesProvidedKey is the
// regression guard for the hardcoded-"id" bug PR-43 review flagged.
// Asserts the SDK request projects the operator-supplied field name,
// not the literal "id".
func TestClient_GetAllDocumentIDsWithPrimaryKey_UsesProvidedKey(t *testing.T) {
	t.Parallel()

	var captured []string
	idx := &fakeIndex{
		getDocumentsFn: func(_ context.Context, q *msdk.DocumentsQuery, resp *msdk.DocumentsResult) error {
			captured = q.Fields
			resp.Results = msdk.Hits{
				{"uid": json.RawMessage(`"doc-1"`)},
			}
			resp.Total = 1
			return nil
		},
	}
	sdk := &fakeSDK{indexFn: func(_ string) msdk.IndexManager { return idx }}
	c := newTestClient(sdk)

	got, err := c.GetAllDocumentIDsWithPrimaryKey(t.Context(), "things", "uid")
	require.NoError(t, err)
	require.Equal(t, []string{"uid"}, captured, "Fields projection must use the supplied primary key — was hardcoded to 'id'")
	require.Equal(t, []string{"doc-1"}, got)
}

// TestClient_GetAllDocumentIDs_DefaultsToIDField confirms the legacy
// no-arg form keeps the previous "id" default — existing callers must
// not need to change after the PR-43 fixes.
func TestClient_GetAllDocumentIDs_DefaultsToIDField(t *testing.T) {
	t.Parallel()

	var captured []string
	idx := &fakeIndex{
		getDocumentsFn: func(_ context.Context, q *msdk.DocumentsQuery, resp *msdk.DocumentsResult) error {
			captured = q.Fields
			resp.Total = 0
			return nil
		},
	}
	sdk := &fakeSDK{indexFn: func(_ string) msdk.IndexManager { return idx }}
	c := newTestClient(sdk)

	_, err := c.GetAllDocumentIDs(t.Context(), "things")
	require.NoError(t, err)
	require.Equal(t, []string{defaultPrimaryKeyField}, captured)
}

// TestClient_DeleteDocumentsByFilter_PassesFilterThrough confirms the
// filter string reaches the SDK unchanged. The SECURITY note in
// DeleteDocumentsByFilter's godoc warns about filter injection — this
// test pins the contract that the filter is opaque to the wrapper.
func TestClient_DeleteDocumentsByFilter_PassesFilterThrough(t *testing.T) {
	t.Parallel()

	var captured any
	idx := &fakeIndex{
		deleteDocumentsByFilterFn: func(_ context.Context, filter any, _ *msdk.DocumentOptions) (*msdk.TaskInfo, error) {
			captured = filter
			return &msdk.TaskInfo{TaskUID: 1}, nil
		},
	}
	sdk := &fakeSDK{indexFn: func(_ string) msdk.IndexManager { return idx }}
	c := newTestClient(sdk)

	const filter = `tenant_id = "abc"`
	_, err := c.DeleteDocumentsByFilter(t.Context(), "things", filter)
	require.NoError(t, err)
	require.Equal(t, filter, captured, "filter must reach the SDK byte-for-byte — no escaping or rewriting is done by this wrapper")
}

// TestClient_FetchDocuments_ReturnsRawHitsAndTotal guards the same
// hit-marshaling loop as Search and pins the Total round-trip:
// FetchResult.Total must mirror DocumentsResult.Total so paginating
// callers can decide when to stop without relying on a short-page signal.
func TestClient_FetchDocuments_ReturnsRawHitsAndTotal(t *testing.T) {
	t.Parallel()

	idx := &fakeIndex{
		getDocumentsFn: func(_ context.Context, _ *msdk.DocumentsQuery, resp *msdk.DocumentsResult) error {
			resp.Results = msdk.Hits{
				{
					"id":    json.RawMessage(`"1"`),
					"title": json.RawMessage(`"alpha"`),
				},
			}
			resp.Total = 42
			return nil
		},
	}
	sdk := &fakeSDK{indexFn: func(_ string) msdk.IndexManager { return idx }}
	c := newTestClient(sdk)

	result, err := c.FetchDocuments(t.Context(), "things", "", 0, 1)
	require.NoError(t, err)
	require.Equal(t, int64(42), result.Total, "Total must mirror DocumentsResult.Total — callers paginate against it")
	require.Len(t, result.Hits, 1)

	var hit map[string]string
	require.NoError(t, json.Unmarshal(result.Hits[0], &hit))
	require.Equal(t, "1", hit["id"])
	require.Equal(t, "alpha", hit["title"])
}

// TestClient_FetchDocuments_PassesFilterAndPaginationThrough pins the
// SECURITY contract (filter is opaque) and the offset/limit parameter
// order — both are int64, so a positional swap would be a silent bug.
func TestClient_FetchDocuments_PassesFilterAndPaginationThrough(t *testing.T) {
	t.Parallel()

	var captured *msdk.DocumentsQuery
	idx := &fakeIndex{
		getDocumentsFn: func(_ context.Context, q *msdk.DocumentsQuery, resp *msdk.DocumentsResult) error {
			captured = q
			resp.Total = 0
			return nil
		},
	}
	sdk := &fakeSDK{indexFn: func(_ string) msdk.IndexManager { return idx }}
	c := newTestClient(sdk)

	const filter = `tenant_id = "abc"`
	_, err := c.FetchDocuments(t.Context(), "things", filter, 100, 25)
	require.NoError(t, err)
	require.NotNil(t, captured)
	require.Equal(t, filter, captured.Filter, "filter must reach the SDK byte-for-byte")
	require.Equal(t, int64(100), captured.Offset, "offset must map to DocumentsQuery.Offset — call order is (offset, limit)")
	require.Equal(t, int64(25), captured.Limit, "limit must map to DocumentsQuery.Limit — call order is (offset, limit)")
}

// TestClient_FetchDocuments_EmptyFilterOmitsFilter pins the `if filter
// != ""` branch: an empty filter must not populate DocumentsQuery.Filter,
// so the SDK request stays unfiltered instead of asking Meilisearch to
// parse an empty expression.
func TestClient_FetchDocuments_EmptyFilterOmitsFilter(t *testing.T) {
	t.Parallel()

	var captured *msdk.DocumentsQuery
	idx := &fakeIndex{
		getDocumentsFn: func(_ context.Context, q *msdk.DocumentsQuery, resp *msdk.DocumentsResult) error {
			captured = q
			resp.Total = 0
			return nil
		},
	}
	sdk := &fakeSDK{indexFn: func(_ string) msdk.IndexManager { return idx }}
	c := newTestClient(sdk)

	_, err := c.FetchDocuments(t.Context(), "things", "", 0, 10)
	require.NoError(t, err)
	require.NotNil(t, captured)
	require.Nil(t, captured.Filter, "empty filter must leave DocumentsQuery.Filter unset")
}
