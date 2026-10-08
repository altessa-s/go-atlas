// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package projectionit_test

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mongoopts "go.mongodb.org/mongo-driver/v2/mongo/options"

	datamongo "github.com/altessa-s/go-atlas/data/mongo"
	"github.com/altessa-s/go-atlas/data/projection"
	projmongo "github.com/altessa-s/go-atlas/data/projection/translators/mongo"
)

const docCount = 25

// envOr returns the environment override for a connection setting, or the
// default that matches tests/integration/docker-compose.yml.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// uniqueSuffix names a throwaway database or table so two runs never collide.
func uniqueSuffix() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

// mongoCollection connects, creates a throwaway database and seeds docCount
// users. It skips when the server is unreachable.
func mongoCollection(t *testing.T) *mongo.Collection {
	t.Helper()

	// directConnection: the compose service is a single-node replica set that
	// advertises its in-container address; see tests/integration/README.md.
	uri := envOr("MONGO_URI", "mongodb://127.0.0.1:27019/?directConnection=true")
	client, err := mongo.Connect(mongoopts.Client().ApplyURI(uri))
	if err != nil {
		t.Skipf("mongodb not available: %v", err)
	}
	if err = client.Ping(t.Context(), nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Skipf("mongodb not reachable: %v", err)
	}
	db := client.Database("projection_it_" + uniqueSuffix())
	t.Cleanup(func() {
		ctx := context.Background()
		_ = db.Drop(ctx)
		_ = client.Disconnect(ctx)
	})

	coll := db.Collection("users")
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	docs := make([]any, 0, docCount)
	for i := range docCount {
		docs = append(docs, bson.M{
			"_id":        bson.NewObjectID(), // ListCursor resumes on ObjectID cursors
			"key":        fmt.Sprintf("u%02d", i),
			"name":       fmt.Sprintf("user %d", i),
			"email":      fmt.Sprintf("u%d@example.com", i),
			"password":   "secret",
			"created_at": base.Add(time.Duration(i%5) * time.Hour), // ties exercise the _id tiebreaker
			"address":    bson.M{"city": "Belgrade", "zip": "11000", "geo": bson.M{"lat": 44.8, "lng": 20.4}},
			"credentials": bson.M{
				"login":    fmt.Sprintf("login%d", i),
				"password": "hash",
			},
		})
	}
	_, err = coll.InsertMany(t.Context(), docs)
	require.NoError(t, err)
	return coll
}

func newMongoTranslator(t *testing.T, opts ...projection.TranslatorOption) *projmongo.Translator {
	t.Helper()
	tr, err := projmongo.NewTranslator(opts...)
	require.NoError(t, err)
	return tr
}

func findOne(t *testing.T, coll *mongo.Collection, proj bson.M) bson.M {
	t.Helper()
	var doc bson.M
	err := coll.FindOne(t.Context(), bson.M{"key": "u01"}, mongoopts.FindOne().SetProjection(proj)).Decode(&doc)
	require.NoError(t, err)
	return doc
}

// subdoc returns the embedded document doc[key] as a map; the driver decodes
// nested documents of a bson.M as bson.D.
func subdoc(t *testing.T, doc bson.M, key string) map[string]any {
	t.Helper()
	d, ok := doc[key].(bson.D)
	require.True(t, ok, "%s is %T", key, doc[key])
	out := make(map[string]any, len(d))
	for _, e := range d {
		out[e.Key] = e.Value
	}
	return out
}

func TestMongoInclusionAcceptedByServer(t *testing.T) {
	t.Parallel()
	coll := mongoCollection(t)

	parser, err := projection.NewParser()
	require.NoError(t, err)
	tr := newMongoTranslator(t,
		projection.WithUntrustedInput(),
		projection.WithAllowedFields("name", "email", "address", "address.*", "credentials.*"),
		projection.WithDeniedFields("credentials.password"),
		projection.WithDefaultFields("name"),
	)

	// The parent and child would be a "Path collision" if sent as is.
	proj, err := tr.Translate(parser.MustParse("name, address, address.geo.lat, credentials.login"))
	require.NoError(t, err)

	doc := findOne(t, coll, proj)
	require.Equal(t, "user 1", doc["name"])
	require.NotContains(t, doc, "_id", "_id is suppressed unless required")
	require.NotContains(t, doc, "email")
	require.NotContains(t, doc, "password")
	require.Equal(t, "11000", subdoc(t, doc, "address")["zip"], "the whole parent survives the collapse")
	require.Equal(t, map[string]any{"login": "login1"}, subdoc(t, doc, "credentials"))

	_, err = tr.Translate(parser.MustParse("credentials"))
	require.ErrorIs(t, err, projection.ErrFieldNotAllowed)
}

func TestMongoExclusionDefault(t *testing.T) {
	t.Parallel()
	coll := mongoCollection(t)

	tr := newMongoTranslator(t,
		projection.WithDeniedFields("password"),
		projection.WithDeniedStorageFields("credentials.password"),
	)
	proj, err := tr.Translate(projection.Spec{})
	require.NoError(t, err)

	doc := findOne(t, coll, proj)
	require.Contains(t, doc, "_id", "an exclusion keeps _id")
	require.Equal(t, "user 1", doc["name"])
	require.NotContains(t, doc, "password")
	require.Equal(t, map[string]any{"login": "login1"}, subdoc(t, doc, "credentials"))
}

type listedUser struct {
	ID        bson.ObjectID `bson:"_id"`
	Name      string        `bson:"name"`
	Email     string        `bson:"email,omitempty"`
	CreatedAt time.Time     `bson:"created_at"`
}

func TestMongoListCursorPagesUnderProjection(t *testing.T) {
	t.Parallel()
	coll := mongoCollection(t)

	sort := bson.D{{Key: "created_at", Value: 1}}
	tr := newMongoTranslator(t,
		projection.WithAllowedFields("name", "email"),
		projection.WithRequiredFields("_id", "created_at"),
	)
	proj, err := tr.Translate(projection.Spec{Paths: []string{"name"}})
	require.NoError(t, err)

	var (
		seen   []string
		cursor *string
	)
	for page := 0; ; page++ {
		require.Less(t, page, docCount, "pagination does not terminate")
		opts := []datamongo.ListCursorOption{
			datamongo.WithListCursorIdField("_id"),
			datamongo.WithListCursorSort(sort),
			datamongo.WithListCursorProjection(proj),
			datamongo.WithListCursorLimit(4),
		}
		if cursor != nil {
			opts = append(opts, datamongo.WithListCursorCursor(cursor))
		}
		res, err := datamongo.ListCursor[listedUser](t.Context(), coll, opts...)
		require.NoError(t, err)
		for _, u := range res.Items {
			require.Empty(t, u.Email, "email was not requested")
			require.NotEmpty(t, u.Name)
			seen = append(seen, u.ID.Hex())
		}
		if res.NextCursor == nil {
			break
		}
		cursor = res.NextCursor
	}
	slices.Sort(seen)
	require.Len(t, seen, docCount)
	require.Len(t, slices.Compact(seen), docCount, "no document is skipped or repeated")
}

func TestMongoListCursorRejectsProjectionDroppingSortKey(t *testing.T) {
	t.Parallel()
	coll := mongoCollection(t)

	tr := newMongoTranslator(t, projection.WithRequiredFields("_id"))
	proj, err := tr.Translate(projection.Spec{Paths: []string{"name"}})
	require.NoError(t, err)

	_, err = datamongo.ListCursor[listedUser](t.Context(), coll,
		datamongo.WithListCursorIdField("_id"),
		datamongo.WithListCursorSort(bson.D{{Key: "created_at", Value: 1}}),
		datamongo.WithListCursorProjection(proj),
	)
	require.ErrorIs(t, err, datamongo.ErrProjectionDropsCursorField)
}

func TestMongoListEmptyProjection(t *testing.T) {
	t.Parallel()
	coll := mongoCollection(t)

	res, err := datamongo.ListCursor[listedUser](t.Context(), coll,
		datamongo.WithListCursorIdField("_id"),
		datamongo.WithListCursorProjection(bson.M{}),
		datamongo.WithListCursorLimit(2),
	)
	require.NoError(t, err, "an empty projection must not reach the server as $project: {}")
	require.Len(t, res.Items, 2)
	require.NotEmpty(t, res.Items[0].Email)
}
