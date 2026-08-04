// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filterit_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	meilitrans "github.com/altessa-s/go-atlas/data/filter/translators/meili"
	"github.com/altessa-s/go-atlas/data/meilisearch"
	"github.com/altessa-s/go-atlas/tests/integration/filterit"
)

// searchLimit is comfortably above the seven-row dataset; Meilisearch
// defaults to 20 hits and would otherwise silently truncate a larger one.
const searchLimit = 100

type meiliBackend struct {
	client *meilisearch.Client
	index  string
}

func (*meiliBackend) name() filterit.Backend { return filterit.Meili }

func (b *meiliBackend) setup(tb testing.TB) {
	tb.Helper()

	host := envOr("MEILI_URL", "http://127.0.0.1:17700")
	key := envOr("MEILI_KEY", "atlas-master-key")

	client, err := meilisearch.New(tb.Context(), host, meilisearch.WithAPIKey(key))
	if err != nil {
		tb.Skipf("meilisearch not reachable: %v", err)
	}

	// CONTAINS and STARTS WITH are gated behind an experimental flag in
	// Meilisearch 1.11; without it the server rejects those filters
	// outright, which would look like a translator bug.
	enableMeiliContainsFilter(tb, host, key)

	b.client = client
	b.index = "filter_it_" + uniqueSuffix()

	require.NoError(tb, client.EnsureIndex(tb.Context(), b.index, "id", &meilisearch.IndexSettings{
		SearchableAttributes: []string{"name", "status"},
		FilterableAttributes: []string{
			"id", "name", "age", "price", "active", "status", "role", "createdAt", "deletedAt",
		},
	}))

	tb.Cleanup(func() {
		_, _ = client.DeleteIndex(context.Background(), b.index)
		client.Close()
	})

	b.seed(tb)
}

// enableMeiliContainsFilter flips the experimental containsFilter switch.
// The wrapper does not expose the endpoint, and it is a single PATCH, so
// the request is made directly.
func enableMeiliContainsFilter(tb testing.TB, host, key string) {
	tb.Helper()

	req, err := http.NewRequestWithContext(tb.Context(), http.MethodPatch,
		strings.TrimRight(host, "/")+"/experimental-features",
		strings.NewReader(`{"containsFilter": true}`))
	require.NoError(tb, err)

	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(tb, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(tb, http.StatusOK, resp.StatusCode, "could not enable the containsFilter experimental feature")
}

func (b *meiliBackend) seed(tb testing.TB) {
	tb.Helper()

	docs := make([]map[string]any, 0, len(filterit.Dataset()))
	for _, row := range filterit.Dataset() {
		doc := map[string]any{
			"id":     row.ID,
			"name":   row.Name,
			"age":    row.Age,
			"price":  row.Price,
			"active": row.Active,
			"status": row.Status,
			"role":   row.Role,
			// The Meilisearch translator renders a timestamp as Unix
			// seconds, so the attribute has to be numeric for the
			// comparison to mean anything.
			"createdAt": row.CreatedAt.Unix(),
		}
		if row.DeletedAt != nil {
			doc["deletedAt"] = row.DeletedAt.Unix()
		}
		docs = append(docs, doc)
	}

	task, err := b.client.IndexDocuments(tb.Context(), b.index, docs)
	require.NoError(tb, err)
	require.NoError(tb, b.client.WaitForTask(tb.Context(), task, 0))
}

func (b *meiliBackend) translator(tb testing.TB, opts ...filter.TranslatorOption) *meilitrans.Translator {
	tb.Helper()

	base := []filter.TranslatorOption{filter.WithFieldMapping(filterit.DocFieldMapping())}
	tr, err := meilitrans.NewTranslator(append(base, opts...)...)
	require.NoError(tb, err)
	return tr
}

func (b *meiliBackend) search(tb testing.TB, expr string) ([]int64, error) {
	tb.Helper()

	clause, err := b.translator(tb).Translate(parse(tb, expr))
	if err != nil {
		return nil, err
	}

	result, err := b.client.Search(tb.Context(), &meilisearch.SearchRequest{
		IndexName: b.index,
		Filter:    clause,
		Limit:     searchLimit,
	})
	if err != nil {
		return nil, err
	}

	ids := make([]int64, 0, len(result.Hits))
	for _, hit := range result.Hits {
		var doc struct {
			ID int64 `json:"id"`
		}
		if err = json.Unmarshal(hit, &doc); err != nil {
			return nil, err
		}
		ids = append(ids, doc.ID)
	}
	return ids, nil
}

func (b *meiliBackend) translate(tb testing.TB, expr string, opts ...filter.TranslatorOption) error {
	tb.Helper()

	_, err := b.translator(tb, opts...).Translate(parse(tb, expr))
	return err
}

func TestMeili(t *testing.T) {
	runCorpus(t, &meiliBackend{})
}

func TestMeili_Rejections(t *testing.T) {
	runRejections(t, &meiliBackend{})
}

// TestMeili_NullFilterSemantics is the regression guard for the null
// handling, kept separate from the corpus because the naive rendering
// fails in a direction the corpus alone would not make obvious.
//
// Meilisearch splits two questions CEL's `null` conflates: EXISTS asks
// whether the attribute is present, IS NULL asks whether it is present
// AND null. With the document-store-natural shape used here — the
// attribute omitted where there is no value, exactly as the MongoDB
// backend stores it — a bare `IS NOT NULL` matches every document,
// including the ones that never had the attribute.
//
// That is the shape of a soft-delete filter, so getting it wrong returns
// the deleted documents. The translator therefore pairs each half with
// the corresponding existence check.
func TestMeili_NullFilterSemantics(t *testing.T) {
	b := &meiliBackend{}
	b.setup(t)

	t.Run("equal null matches the documents without the attribute", func(t *testing.T) {
		got, err := b.search(t, `deletedAt == null`)
		require.NoError(t, err)
		require.Equal(t, []int64{1, 2, 3, 4, 5, 7}, normalize(got))
	})

	t.Run("not equal null matches only the document that has a value", func(t *testing.T) {
		got, err := b.search(t, `deletedAt != null`)
		require.NoError(t, err)
		require.Equal(t, []int64{6}, normalize(got),
			"a soft-delete filter written as `deletedAt != null` must not leak the other documents")
	})

	t.Run("has agrees with not-equal-null", func(t *testing.T) {
		got, err := b.search(t, `has(row.deletedAt)`)
		require.NoError(t, err)
		require.Equal(t, []int64{6}, normalize(got))
	})

	// The bare form the translator no longer emits. Asserting it here
	// records why the parenthesized pair exists: without the EXISTS
	// half, this is what the filter would have returned.
	t.Run("the bare IS NOT NULL it replaced would match everything", func(t *testing.T) {
		result, err := b.client.Search(t.Context(), &meilisearch.SearchRequest{
			IndexName: b.index,
			Filter:    "deletedAt IS NOT NULL",
			Limit:     searchLimit,
		})
		require.NoError(t, err)
		require.Len(t, result.Hits, len(filterit.Dataset()))
	})
}
