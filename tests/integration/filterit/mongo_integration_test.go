// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package filterit_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mongoopts "go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/altessa-s/go-atlas/data/filter"
	mongotrans "github.com/altessa-s/go-atlas/data/filter/translators/mongo"
	"github.com/altessa-s/go-atlas/tests/integration/filterit"
)

type mongoBackend struct {
	client *mongo.Client
	coll   *mongo.Collection
}

func (*mongoBackend) name() filterit.Backend { return filterit.Mongo }

func (b *mongoBackend) setup(tb testing.TB) {
	tb.Helper()

	uri := envOr("MONGO_URI", "mongodb://127.0.0.1:27019")
	client, err := mongo.Connect(mongoopts.Client().ApplyURI(uri))
	if err != nil {
		tb.Skipf("mongodb not available: %v", err)
	}
	if err = client.Ping(tb.Context(), nil); err != nil {
		_ = client.Disconnect(context.Background())
		tb.Skipf("mongodb not reachable: %v", err)
	}

	db := client.Database("filter_it_" + uniqueSuffix())
	b.client = client
	b.coll = db.Collection("rows")

	tb.Cleanup(func() {
		ctx := context.Background()
		_ = db.Drop(ctx)
		_ = client.Disconnect(ctx)
	})

	b.seed(tb)
}

func (b *mongoBackend) seed(tb testing.TB) {
	tb.Helper()

	docs := make([]any, 0, len(filterit.Dataset()))
	for _, row := range filterit.Dataset() {
		doc := bson.M{
			"id":        row.ID,
			"name":      row.Name,
			"age":       row.Age,
			"price":     row.Price,
			"active":    row.Active,
			"status":    row.Status,
			"role":      row.Role,
			"createdAt": row.CreatedAt,
		}
		// Left absent rather than stored as null, which is how a
		// document store normally represents "no value" — and what the
		// `== null` / has() cases are written against.
		if row.DeletedAt != nil {
			doc["deletedAt"] = *row.DeletedAt
		}
		docs = append(docs, doc)
	}

	_, err := b.coll.InsertMany(tb.Context(), docs)
	require.NoError(tb, err)
}

func (b *mongoBackend) translator(tb testing.TB, opts ...filter.TranslatorOption) *mongotrans.Translator {
	tb.Helper()

	base := []filter.TranslatorOption{filter.WithFieldMapping(filterit.DocFieldMapping())}
	tr, err := mongotrans.NewTranslator(append(base, opts...)...)
	require.NoError(tb, err)
	return tr
}

func (b *mongoBackend) search(tb testing.TB, expr string) ([]int64, error) {
	tb.Helper()

	query, err := b.translator(tb).Translate(parse(tb, expr))
	if err != nil {
		return nil, err
	}

	cursor, err := b.coll.Find(tb.Context(), query,
		mongoopts.Find().SetSort(bson.D{{Key: "id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(context.Background()) }()

	var ids []int64
	for cursor.Next(tb.Context()) {
		var doc struct {
			ID int64 `bson:"id"`
		}
		if err = cursor.Decode(&doc); err != nil {
			return nil, err
		}
		ids = append(ids, doc.ID)
	}
	return ids, cursor.Err()
}

func (b *mongoBackend) translate(tb testing.TB, expr string, opts ...filter.TranslatorOption) error {
	tb.Helper()

	_, err := b.translator(tb, opts...).Translate(parse(tb, expr))
	return err
}

func TestMongo(t *testing.T) {
	runCorpus(t, &mongoBackend{})
}

func TestMongo_Rejections(t *testing.T) {
	runRejections(t, &mongoBackend{})
}
