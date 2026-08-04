// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filterit_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/filter"
	rstrans "github.com/altessa-s/go-atlas/data/filter/translators/redisearch"
	"github.com/altessa-s/go-atlas/tests/integration/filterit"
)

// redisearchSchema is the index definition and, in the same breath, the
// schema the translator is handed: a field's RediSearch type decides
// whether a comparison becomes a numeric range or a tag match, so the two
// must agree or every query is silently wrong.
func redisearchSchema() map[string]rstrans.FieldType {
	return map[string]rstrans.FieldType{
		"id":        rstrans.FieldTypeNumeric,
		"name":      rstrans.FieldTypeText,
		"age":       rstrans.FieldTypeNumeric,
		"price":     rstrans.FieldTypeNumeric,
		"active":    rstrans.FieldTypeTag,
		"status":    rstrans.FieldTypeTag,
		"role":      rstrans.FieldTypeNumeric,
		"createdAt": rstrans.FieldTypeNumeric,
	}
}

type redisearchBackend struct {
	rdb    *redis.Client
	index  string
	prefix string
}

func (*redisearchBackend) name() filterit.Backend { return filterit.RediSearch }

func (b *redisearchBackend) setup(tb testing.TB) {
	tb.Helper()

	rdb := redis.NewClient(&redis.Options{Addr: envOr("REDIS_ADDR", "127.0.0.1:16379")})
	if err := rdb.Ping(tb.Context()).Err(); err != nil {
		_ = rdb.Close()
		tb.Skipf("redis not reachable: %v", err)
	}

	b.rdb = rdb
	suffix := uniqueSuffix()
	b.index = "filter_it_" + suffix
	b.prefix = "filter_it_" + suffix + ":"

	// WithSuffixtrie is what makes the infix query contains() emits
	// (`@name:*li*`) resolvable; without it RediSearch returns nothing.
	err := rdb.FTCreate(tb.Context(), b.index,
		&redis.FTCreateOptions{OnHash: true, Prefix: []any{b.prefix}},
		&redis.FieldSchema{FieldName: "id", FieldType: redis.SearchFieldTypeNumeric, Sortable: true},
		&redis.FieldSchema{FieldName: "name", FieldType: redis.SearchFieldTypeText, WithSuffixtrie: true},
		&redis.FieldSchema{FieldName: "age", FieldType: redis.SearchFieldTypeNumeric},
		&redis.FieldSchema{FieldName: "price", FieldType: redis.SearchFieldTypeNumeric},
		&redis.FieldSchema{FieldName: "active", FieldType: redis.SearchFieldTypeTag},
		&redis.FieldSchema{FieldName: "status", FieldType: redis.SearchFieldTypeTag},
		&redis.FieldSchema{FieldName: "role", FieldType: redis.SearchFieldTypeNumeric},
		&redis.FieldSchema{FieldName: "createdAt", FieldType: redis.SearchFieldTypeNumeric},
	).Err()
	if err != nil {
		_ = rdb.Close()
		tb.Skipf("redisearch not available (needs Redis Stack): %v", err)
	}

	tb.Cleanup(func() {
		ctx := context.Background()
		_ = rdb.FTDropIndexWithArgs(ctx, b.index, &redis.FTDropIndexOptions{DeleteDocs: true}).Err()
		_ = rdb.Close()
	})

	b.seed(tb)
}

func (b *redisearchBackend) seed(tb testing.TB) {
	tb.Helper()

	for _, row := range filterit.Dataset() {
		key := b.prefix + strconv.FormatInt(row.ID, 10)
		err := b.rdb.HSet(tb.Context(), key,
			"id", row.ID,
			"name", row.Name,
			"age", row.Age,
			"price", strconv.FormatFloat(row.Price, 'f', -1, 64),
			"active", strconv.FormatBool(row.Active),
			"status", row.Status,
			"role", row.Role,
			"createdAt", row.CreatedAt.Unix(),
		).Err()
		require.NoError(tb, err)
	}
}

func (b *redisearchBackend) translator(tb testing.TB, opts ...filter.TranslatorOption) *rstrans.Translator {
	tb.Helper()

	tr, err := rstrans.NewTranslator(redisearchSchema(), opts...)
	require.NoError(tb, err)
	return tr
}

func (b *redisearchBackend) search(tb testing.TB, expr string) ([]int64, error) {
	tb.Helper()

	query, err := b.translator(tb).Translate(parse(tb, expr))
	if err != nil {
		return nil, err
	}

	result, err := b.rdb.FTSearchWithArgs(tb.Context(), b.index, query,
		&redis.FTSearchOptions{LimitOffset: 0, Limit: searchLimit}).Result()
	if err != nil {
		return nil, err
	}

	ids := make([]int64, 0, len(result.Docs))
	for _, doc := range result.Docs {
		id, convErr := strconv.ParseInt(doc.Fields["id"], 10, 64)
		if convErr != nil {
			return nil, convErr
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (b *redisearchBackend) translate(tb testing.TB, expr string, opts ...filter.TranslatorOption) error {
	tb.Helper()

	_, err := b.translator(tb, opts...).Translate(parse(tb, expr))
	return err
}

func TestRediSearch(t *testing.T) {
	runCorpus(t, &redisearchBackend{})
}

func TestRediSearch_Rejections(t *testing.T) {
	runRejections(t, &redisearchBackend{})
}
