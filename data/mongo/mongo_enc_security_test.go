// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"

	kmslocal "github.com/altessa-s/go-atlas/data/mongo/kms/local"
)

// encryptedSubtype is the BSON binary subtype of a CSFLE ciphertext.
const encryptedSubtype = 6

// fakeClientEncryption is a scripted clientEncryption: lookups pop results
// from keyLookups in order, CreateDataKey returns createID/createErr, and
// Encrypt records each plaintext and returns a subtype-6 binary.
type fakeClientEncryption struct {
	mu         sync.Mutex
	keyLookups []*mongo.SingleResult
	createID   bson.Binary
	createErr  error
	encrypted  []bson.RawValue
}

func (f *fakeClientEncryption) GetKeyByAltName(context.Context, string) *mongo.SingleResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	res := f.keyLookups[0]
	f.keyLookups = f.keyLookups[1:]
	return res
}

func (f *fakeClientEncryption) CreateDataKey(context.Context, string, ...mongoOptions.Lister[mongoOptions.DataKeyOptions]) (bson.Binary, error) {
	return f.createID, f.createErr
}

func (f *fakeClientEncryption) Encrypt(_ context.Context, val bson.RawValue, _ ...mongoOptions.Lister[mongoOptions.EncryptOptions]) (bson.Binary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.encrypted = append(f.encrypted, val)
	return bson.Binary{Subtype: encryptedSubtype, Data: []byte{0xE}}, nil
}

func (f *fakeClientEncryption) Close(context.Context) error { return nil }

// newEncryptingMongo returns a Mongo whose encryption is configured and
// served by ce, without a server or libmongocrypt.
func newEncryptingMongo(t *testing.T, ce clientEncryption) *Mongo {
	t.Helper()
	kms, err := kmslocal.New(kmslocal.WithMasterKey(strings.Repeat("k", kmslocal.RequiredMasterKeyLength)))
	require.NoError(t, err)
	m, err := New("testdb")
	require.NoError(t, err)
	m.config.EncryptionEnabled = true
	m.config.KMS = kms
	m.encryptionClient = ce
	return m
}

func keyDoc(id bson.Binary) *mongo.SingleResult {
	return mongo.NewSingleResultFromDocument(bson.D{{Key: "_id", Value: id}}, nil, nil)
}

func noKeyDoc() *mongo.SingleResult {
	return mongo.NewSingleResultFromDocument(bson.D{}, mongo.ErrNoDocuments, nil)
}

type excludedFieldEntity struct {
	ID     string  `bson:"_id"`
	Name   string  `bson:"name"`
	Secret string  `bson:"-"`
	Token  *string `bson:"-"`
}

func TestConvert_BSONExcludedFieldsNotPersisted(t *testing.T) {
	t.Parallel()

	m, err := New("testdb")
	require.NoError(t, err)

	entity := &excludedFieldEntity{ID: "x1", Name: "visible", Secret: "s3cret"}

	doc, err := m.ConvertToNewDocument(t.Context(), entity)
	require.NoError(t, err)
	require.Equal(t, "visible", doc["name"])
	require.NotContains(t, doc, "secret")
	require.NotContains(t, doc, "token")

	upd, err := m.ConvertToUpdateDocument(t.Context(), entity)
	require.NoError(t, err)
	set, ok := upd["$set"].(bson.M)
	require.True(t, ok, "expected $set document")
	require.Equal(t, "visible", set["name"])
	require.NotContains(t, set, "secret")
	unset, _ := upd["$unset"].(bson.M)
	require.NotContains(t, unset, "token", "a nil excluded pointer must not be unset either")
}

// ExcludedEmbedded is exported so the embedded field is reachable via reflection.
type ExcludedEmbedded struct {
	Internal string `bson:"internal"`
}

// IncludedEmbedded is exported so the embedded field is reachable via reflection.
type IncludedEmbedded struct {
	Shared string `bson:"shared"`
}

type excludedEmbeddedEntity struct {
	ID                string `bson:"_id"`
	ExcludedEmbedded  `bson:"-"`
	*IncludedEmbedded `bson:"-"`
	Name              string `bson:"name"`
}

func TestConvert_BSONExcludedEmbeddedStructNotPersisted(t *testing.T) {
	t.Parallel()

	m, err := New("testdb")
	require.NoError(t, err)

	entity := &excludedEmbeddedEntity{
		ID:               "x2",
		ExcludedEmbedded: ExcludedEmbedded{Internal: "hidden"},
		IncludedEmbedded: &IncludedEmbedded{Shared: "hidden-too"},
		Name:             "visible",
	}

	doc, err := m.ConvertToNewDocument(t.Context(), entity)
	require.NoError(t, err)
	require.Equal(t, "visible", doc["name"])
	require.NotContains(t, doc, "internal")
	require.NotContains(t, doc, "shared")

	upd, err := m.ConvertToUpdateDocument(t.Context(), entity)
	require.NoError(t, err)
	set, ok := upd["$set"].(bson.M)
	require.True(t, ok, "expected $set document")
	require.NotContains(t, set, "internal")
	require.NotContains(t, set, "shared")
}

type encryptedChild struct {
	Value string `bson:"value"`
}

type encryptedFieldsEntity struct {
	ID       string            `bson:"_id"`
	Flag     bool              `bson:"flag" encryption:"deterministic,k"`
	Empty    string            `bson:"empty" encryption:"deterministic,k"`
	Zero     int               `bson:"zero" encryption:"deterministic,k"`
	Tags     []string          `bson:"tags" encryption:"random,k"`
	Labels   map[string]string `bson:"labels" encryption:"random,k"`
	Child    *encryptedChild   `bson:"child" encryption:"random,k"`
	Optional *string           `bson:"optional" encryption:"deterministic,k"`
}

func TestConvert_EncryptedFieldsFailClosedWithoutEncryption(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		entity any
	}{
		{"bool true", &struct {
			F bool `bson:"f" encryption:"deterministic,k"`
		}{F: true}},
		{"bool false", &struct {
			F bool `bson:"f" encryption:"deterministic,k"`
		}{}},
		{"empty string", &struct {
			F string `bson:"f" encryption:"deterministic,k"`
		}{}},
		{"zero int", &struct {
			F int `bson:"f" encryption:"deterministic,k"`
		}{}},
		{"string slice", &struct {
			F []string `bson:"f" encryption:"random,k"`
		}{F: []string{"a"}}},
		{"map", &struct {
			F map[string]string `bson:"f" encryption:"random,k"`
		}{F: map[string]string{"a": "b"}}},
		{"struct pointer", &struct {
			F *encryptedChild `bson:"f" encryption:"random,k"`
		}{F: &encryptedChild{Value: "v"}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m, err := New("testdb")
			require.NoError(t, err)

			_, err = m.ConvertToNewDocument(t.Context(), tc.entity)
			require.ErrorIs(t, err, ErrEncryptionNotEnabled)
			_, err = m.ConvertToUpdateDocument(t.Context(), tc.entity)
			require.ErrorIs(t, err, ErrEncryptionNotEnabled)
		})
	}
}

func TestConvert_EncryptedFieldsAreEncryptedWhateverTheirKind(t *testing.T) {
	t.Parallel()

	ce := &fakeClientEncryption{}
	m := newEncryptingMongo(t, ce)

	entity := &encryptedFieldsEntity{
		ID:     "e1",
		Tags:   []string{"a", "b"},
		Labels: map[string]string{"k": "v"},
		Child:  &encryptedChild{Value: "secret"},
	}

	doc, err := m.ConvertToNewDocument(t.Context(), entity)
	require.NoError(t, err)

	for _, field := range []string{"flag", "empty", "zero", "tags", "labels", "child"} {
		bin, ok := doc[field].(*bson.Binary)
		require.True(t, ok, "%s must be stored as ciphertext, got %T", field, doc[field])
		require.Equal(t, byte(encryptedSubtype), bin.Subtype, field)
	}
	require.Nil(t, doc["optional"], "a nil pointer is stored as null, not encrypted")
	require.Len(t, ce.encrypted, 6, "every non-nil encrypted field goes through Encrypt")
	require.Equal(t, "e1", doc["_id"])
}

func TestCreateDataKey_ConcurrentCreatorReturnsWinnerKey(t *testing.T) {
	t.Parallel()

	winner := bson.Binary{Subtype: bson.TypeBinaryUUID, Data: []byte("0123456789abcdef")}
	ce := &fakeClientEncryption{
		// Not found before creating, then the winner's key after losing.
		keyLookups: []*mongo.SingleResult{noKeyDoc(), keyDoc(winner)},
		createErr:  mongo.WriteException{WriteErrors: []mongo.WriteError{{Code: MongoErrorCodeDuplicateKey}}},
	}
	m := newEncryptingMongo(t, ce)

	got, err := m.CreateDataKey(t.Context(), "k")
	require.NoError(t, err)
	require.Equal(t, DataKeyId(winner), *got)

	cached, err := m.DataKey(t.Context(), "k", false)
	require.NoError(t, err)
	require.Equal(t, DataKeyId(winner), *cached, "the winner's id must be cached, not the driver's zero id")
}

func TestDataKey_RejectsEmptyKeyID(t *testing.T) {
	t.Parallel()

	ce := &fakeClientEncryption{keyLookups: []*mongo.SingleResult{keyDoc(bson.Binary{Subtype: bson.TypeBinaryUUID})}}
	m := newEncryptingMongo(t, ce)

	_, err := m.DataKey(t.Context(), "k", false)
	require.Error(t, err)
	_, cached := m.dkids.Load("k")
	require.False(t, cached, "an empty id must never be cached")
}
