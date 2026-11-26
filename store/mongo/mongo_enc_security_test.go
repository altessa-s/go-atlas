// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-atlas-core/types/optional"
	"github.com/altessa-s/go-atlas/store/mongo/bsoncodec"

	kmslocal "github.com/altessa-s/go-atlas/store/mongo/kms/providers/local"
	mongoOptions "go.mongodb.org/mongo-driver/v2/mongo/options"
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
	encryptErr error
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
	return bson.Binary{Subtype: encryptedSubtype, Data: []byte{0xE}}, f.encryptErr
}

func (f *fakeClientEncryption) Close(context.Context) error { return nil }

// newEncryptingMongo returns a Mongo whose encryption is configured and
// served by ce, without a server or libmongocrypt.
func newEncryptingMongo(t testing.TB, ce clientEncryption, opts ...Option) *Mongo {
	t.Helper()
	kms, err := kmslocal.New(kmslocal.WithMasterKey(strings.Repeat("k", kmslocal.RequiredMasterKeyLength)))
	require.NoError(t, err)
	m, err := New("testdb", append(opts, WithEncryptionEnabled(), WithKMS(kms))...)
	require.NoError(t, err)
	m.encryption.installClient(ce)
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
		Internal:         "hidden",
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
		bin, ok := doc[field].(bson.Binary)
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
	_, cached := m.encryption.current.dkids.Load("k")
	require.False(t, cached, "an empty id must never be cached")
}

type secretChild struct {
	Secret string `bson:"payload" db:"content" encryption:"random,key"`
}

func TestConvert_EncryptedDescendants(t *testing.T) {
	t.Parallel()

	const plaintext = "value that must never appear in encoded BSON"
	type EmbeddedSecret struct {
		Data secretChild `bson:"sub" db:"inner"`
	}
	child := secretChild{Secret: plaintext}
	entity := struct {
		EmbeddedSecret `bson:",inline" db:",inline"`
		Point          *secretChild
		List           []*secretChild
		Map            map[string]secretChild
		Optional       optional.Optional[string] `encryption:"random,key"`
	}{EmbeddedSecret{Data: child}, &child, []*secretChild{&child}, map[string]secretChild{"k": child}, optional.Some(plaintext)}

	for _, tc := range []struct {
		tag, sub, key string
	}{
		{"bson", "sub", "payload"},
		{"db", "inner", "content"},
	} {
		t.Run(tc.tag, func(t *testing.T) {
			t.Parallel()
			m := newEncryptingMongo(t, &fakeClientEncryption{}, WithBSONTagName(tc.tag))
			for _, update := range []bool{false, true} {
				convert := m.ConvertToNewDocument
				if update {
					convert = m.ConvertToUpdateDocument
				}
				doc, err := convert(t.Context(), entity)
				require.NoError(t, err)
				raw, err := bson.Marshal(doc)
				require.NoError(t, err)
				require.False(t, bytes.Contains(raw, []byte(plaintext)))
				stored := bson.Raw(raw)
				if update {
					stored = stored.Lookup("$set").Document()
				}
				for _, path := range [][]string{{tc.sub, tc.key}, {"point", tc.key}, {"list", "0", tc.key}, {"map", "k", tc.key}, {"optional"}} {
					v, err := stored.LookupErr(path...)
					require.NoError(t, err, "%v", path)
					sub, _, ok := v.BinaryOK()
					require.True(t, ok, "%v", path)
					require.Equal(t, byte(encryptedSubtype), sub, "%v", path)
				}
			}
		})
	}
}

type secretMarshaler struct {
	Secret string `encryption:"random,key"`
}

func (s secretMarshaler) MarshalBSON() ([]byte, error) {
	return bson.Marshal(bson.D{{Key: "different", Value: s.Secret}})
}

func TestConvert_RejectsUnplaceableCiphertext(t *testing.T) {
	t.Parallel()
	const plaintext = "must remain secret"
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"opaque struct", struct{ Field secretMarshaler }{secretMarshaler{plaintext}}},
		{"encrypted inline container", struct {
			In secretChild `bson:",inline" encryption:"random,key"`
		}{secretChild{plaintext}}},
		{"encrypted inline map", struct {
			In map[string]string `bson:",inline" encryption:"random,key"`
		}{map[string]string{"k": plaintext}}},
		{"unencodable value", struct {
			Field chan int `encryption:"random,key"`
		}{make(chan int)}},
		{"malformed tag", struct {
			Field string `encryption:"random"`
		}{plaintext}},
		{"opaque Optional descendant", struct {
			In optional.Optional[secretMarshaler]
		}{optional.Some(secretMarshaler{plaintext})}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newEncryptingMongo(t, &fakeClientEncryption{})
			for _, convert := range []func(context.Context, any) (bson.M, error){m.ConvertToNewDocument, m.ConvertToUpdateDocument} {
				doc, err := convert(t.Context(), tc.value)
				require.ErrorIs(t, err, ErrFieldEncryption)
				require.Nil(t, doc)
			}
		})
	}
}

func TestConvert_EncryptorFailureReturnsNoDocument(t *testing.T) {
	t.Parallel()
	cause := errors.New("KMS unavailable")
	m := newEncryptingMongo(t, &fakeClientEncryption{encryptErr: cause})
	for _, convert := range []func(context.Context, any) (bson.M, error){m.ConvertToNewDocument, m.ConvertToUpdateDocument} {
		doc, err := convert(t.Context(), struct {
			Public string
			Child  secretChild
		}{"ordinary value", secretChild{"secret"}})
		require.ErrorIs(t, err, ErrFieldEncryption)
		require.ErrorIs(t, err, cause)
		require.Nil(t, doc)
	}
}

func TestConvert_EncryptsWithClientRegistry(t *testing.T) {
	t.Parallel()
	type token string
	reg := bsoncodec.NewRegistry()
	reg.RegisterTypeEncoder(reflect.TypeFor[token](), bson.ValueEncoderFunc(func(_ bson.EncodeContext, w bson.ValueWriter, _ reflect.Value) error {
		return w.WriteString("from custom client registry")
	}))
	ce := &fakeClientEncryption{}
	m := newEncryptingMongo(t, ce, WithClientOptions(mongoOptions.Client().SetRegistry(reg)))
	entity := struct {
		Field token `encryption:"random,key"`
	}{"original"}
	for _, convert := range []func(context.Context, any) (bson.M, error){m.ConvertToNewDocument, m.ConvertToUpdateDocument} {
		doc, err := convert(t.Context(), entity)
		require.NoError(t, err)
		raw, err := bson.Marshal(doc)
		require.NoError(t, err)
		require.False(t, bytes.Contains(raw, []byte("original")))
		require.False(t, bytes.Contains(raw, []byte("from custom client registry")))
	}
	require.Len(t, ce.encrypted, 2)
	for _, plain := range ce.encrypted {
		require.Equal(t, "from custom client registry", plain.StringValue())
	}
}

// EncryptedRecord is exported so the BSON codec includes the embedded field.
type EncryptedRecord struct {
	Secret string `bson:"renamed" encryption:"unknown-alg,old-key"`
	Owner  string `bson:",omitonupdate"`
}

func TestConvert_EmbeddedModelUsesDeclaringType(t *testing.T) {
	t.Parallel()
	model := &EncryptionModel{
		Type:   EncryptedRecord{},
		Fields: []EncryptionField{{FieldName: "Secret", Algorithm: "random", KeyAltName: "model-key"}},
	}
	m := newEncryptingMongo(t, &fakeClientEncryption{}, WithEncryptionModel(model))
	entity := struct {
		EncryptedRecord `bson:"record"`
	}{EncryptedRecord{Secret: "secret", Owner: "owner"}}
	doc, err := m.ConvertToNewDocument(t.Context(), entity)
	require.NoError(t, err)
	raw, err := bson.Marshal(doc)
	require.NoError(t, err)
	sub, _, ok := bson.Raw(raw).Lookup("record", "renamed").BinaryOK()
	require.True(t, ok)
	require.Equal(t, byte(encryptedSubtype), sub)
	doc, err = m.ConvertToUpdateDocument(t.Context(), entity)
	require.NoError(t, err)
	raw, err = bson.Marshal(doc)
	require.NoError(t, err)
	set := bson.Raw(raw).Lookup("$set").Document()
	sub, _, ok = set.Lookup("record.renamed").BinaryOK()
	require.True(t, ok)
	require.Equal(t, byte(encryptedSubtype), sub)
	_, err = set.LookupErr("record")
	require.Error(t, err)
}

func TestConvertToNewDocument_RootMarshalerCannotBypassEncryption(t *testing.T) {
	t.Parallel()
	m := newEncryptingMongo(t, &fakeClientEncryption{})
	doc, err := m.ConvertToNewDocument(t.Context(), &secretMarshaler{Secret: "secret"})
	require.ErrorIs(t, err, ErrFieldEncryption)
	require.Nil(t, doc)
}

// A custom marshaler may expose a value held behind a private interface.
type wrappedPayload struct {
	payload any
}

func (w wrappedPayload) MarshalBSON() ([]byte, error) {
	return bson.Marshal(bson.D{{Key: "value", Value: w.payload}})
}

func TestConvert_PrivateDynamicPayload(t *testing.T) {
	t.Parallel()
	m := newEncryptingMongo(t, &fakeClientEncryption{})
	for _, convert := range []func(context.Context, any) (bson.M, error){m.ConvertToNewDocument, m.ConvertToUpdateDocument} {
		type record struct{ Wrapper wrappedPayload }
		doc, err := convert(t.Context(), record{wrappedPayload{"plain"}})
		require.NoError(t, err)
		require.NotEmpty(t, doc)
		doc, err = convert(t.Context(), record{wrappedPayload{secretChild{Secret: "secret"}}})
		require.ErrorIs(t, err, ErrFieldEncryption)
		require.Nil(t, doc)
	}
}

func TestConvertToNewDocument_EncryptedSubdocumentOrder(t *testing.T) {
	t.Parallel()
	type child struct {
		Z     string
		Token string `encryption:"random,key"`
		A     string
	}
	m := newEncryptingMongo(t, &fakeClientEncryption{})
	doc, err := m.ConvertToNewDocument(t.Context(), struct{ Children []child }{[]child{{"first", "secret", "last"}}})
	require.NoError(t, err)
	require.Equal(t, bson.A{bson.D{
		{Key: "z", Value: "first"},
		{Key: "token", Value: bson.Binary{Subtype: encryptedSubtype, Data: []byte{0xE}}},
		{Key: "a", Value: "last"},
	}}, doc["children"])
}

func TestConvert_AbsentOptionalWithEncryptedDescendant(t *testing.T) {
	t.Parallel()
	m := newEncryptingMongo(t, &fakeClientEncryption{})
	record := struct {
		Child optional.Optional[secretChild]
	}{}
	for _, update := range []bool{false, true} {
		convert := m.ConvertToNewDocument
		if update {
			convert = m.ConvertToUpdateDocument
		}
		doc, err := convert(t.Context(), record)
		require.NoError(t, err)
		raw, err := bson.Marshal(doc)
		require.NoError(t, err)
		stored := bson.Raw(raw)
		if update {
			stored = stored.Lookup("$set").Document()
		}
		require.Equal(t, bson.TypeNull, stored.Lookup("child").Type)
	}
}

func TestEncrypt_UsesRegisteredValueEncoder(t *testing.T) {
	t.Parallel()
	reg := bsoncodec.NewRegistry()
	reg.RegisterTypeEncoder(reflect.TypeFor[secretMarshaler](), bson.ValueEncoderFunc(func(_ bson.EncodeContext, w bson.ValueWriter, _ reflect.Value) error {
		return w.WriteString("registered value")
	}))
	ce := &fakeClientEncryption{}
	m := newEncryptingMongo(t, ce, WithClientOptions(mongoOptions.Client().SetRegistry(reg)))
	m.encryption.current.dkids.Store("key", DataKeyId{Subtype: bson.TypeBinaryUUID, Data: []byte("0123456789abcdef")})
	_, err := m.Encrypt(t.Context(), secretMarshaler{Secret: "value"}, EncryptionAlgRandom, "key")
	require.NoError(t, err)
	require.Len(t, ce.encrypted, 1)
	require.Equal(t, bson.TypeString, ce.encrypted[0].Type)
	require.Equal(t, "registered value", ce.encrypted[0].StringValue())
}

func TestConvert_ValidatesValueBeforeFieldEncoding(t *testing.T) {
	t.Parallel()
	var value any = "leaf"
	for range 200 {
		value = bson.M{"child": value}
	}
	m := newEncryptingMongo(t, &fakeClientEncryption{})
	for _, convert := range []func(context.Context, any) (bson.M, error){m.ConvertToNewDocument, m.ConvertToUpdateDocument} {
		doc, err := convert(t.Context(), struct {
			Value any `encryption:"random,key"`
		}{value})
		require.ErrorIs(t, err, ErrFieldEncryption)
		require.Nil(t, doc)
	}
}

func TestNewRejectsOverlappingEncryptionModels(t *testing.T) {
	t.Parallel()
	type child struct{ Secret string }
	type middle struct{ Children map[string][]*child }
	type parent struct{ Outer middle }
	type tagged struct {
		Outer middle `encryption:"random,outer"`
	}
	model := func(value any, field, key string) *EncryptionModel {
		return &EncryptionModel{Type: value, Fields: []EncryptionField{{FieldName: field, Algorithm: "random", KeyAltName: key}}}
	}

	for _, tc := range []struct {
		name  string
		outer *EncryptionModel
	}{
		{"model and model", model(parent{}, "Outer", "outer")},
		{"tag and model", &EncryptionModel{Type: tagged{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, err := New("testdb", WithEncryptionModel(tc.outer, model(child{}, "Secret", "inner")))
			require.ErrorIs(t, err, ErrFieldEncryption)
			require.Nil(t, m)
		})
	}
}

func TestConvertRejectsOverlappingEncryptionRules(t *testing.T) {
	t.Parallel()
	type child struct {
		Secret string `encryption:"unknown,inner"`
	}
	type static struct {
		Outer *child `bson:",omitempty" encryption:"random,outer"`
	}
	type dynamic struct {
		Outer any `encryption:"random,outer"`
	}

	for _, tc := range []struct {
		name  string
		value any
	}{
		{"static tags", static{&child{"secret"}}},
		{"omitted field", static{}},
		{"dynamic collection", dynamic{map[string][]any{"k": {&child{"secret"}}}}},
		{"dynamic typed nil", dynamic{(*child)(nil)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ce := &fakeClientEncryption{}
			m := newEncryptingMongo(t, ce)
			for _, convert := range []func(context.Context, any) (bson.M, error){m.ConvertToNewDocument, m.ConvertToUpdateDocument} {
				doc, err := convert(t.Context(), tc.value)
				require.ErrorIs(t, err, ErrFieldEncryption)
				require.Nil(t, doc)
				require.Empty(t, ce.encrypted)
			}
		})
	}
}
