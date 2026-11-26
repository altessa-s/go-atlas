// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/altessa-s/go-aip/pagination/pagetoken"
)

// newCursorSigner returns a page-token codec over a fixed test key.
func newCursorSigner(t testing.TB, key byte, opts ...pagetoken.Option) *pagetoken.Codec {
	t.Helper()
	codec, err := pagetoken.New(bytes.Repeat([]byte{key}, pagetoken.MinKeyLength), opts...)
	require.NoError(t, err)
	return codec
}

// mintSignedCursor issues a "next" token the way ListCursor does for a page
// of collection read with o.
func mintSignedCursor(t testing.TB, collection *mongo.Collection, o ...ListCursorOption) string {
	t.Helper()
	opts := newListCursorOptions(o...)
	token, err := opts.nextCursorToken(t.Context(), bson.NewObjectID().Hex(), "alice",
		cursorCollectionDigest(collection))
	require.NoError(t, err)
	return token
}

// signedRequest is the request the signed-cursor tests page with.
func signedRequest(signer CursorSigner, subject string) []ListCursorOption {
	return []ListCursorOption{
		WithListCursorSigner(signer),
		WithListCursorSubject(subject),
		WithListCursorFilter(bson.M{"status": "active"}),
		WithListCursorSort(bson.D{{Key: "name", Value: 1}}),
	}
}

// TestSignedCursor_RoundTrip pins that a signed token resolves under the
// request it was issued for, bound to a subject or not, and yields the cursor
// it was minted from.
func TestSignedCursor_RoundTrip(t *testing.T) {
	t.Parallel()

	coll := newDetachedCollection(t)
	for _, subject := range []string{"", "tenant-A"} {
		t.Run("subject="+subject, func(t *testing.T) {
			t.Parallel()
			request := signedRequest(newCursorSigner(t, 1), subject)
			token := mintSignedCursor(t, coll, request...)
			require.False(t, isULID(token))

			opts := newListCursorOptions(append(request, WithListCursorCursor(token))...)
			require.NoError(t, resolveCursorOptions(t.Context(), opts, cursorCollectionDigest(coll)))
			require.NotNil(t, opts.cursor)
			require.True(t, isValidCursorId(opts.cursor.CursorId))
			value, err := decodeSortValue(opts.cursor.SortValue)
			require.NoError(t, err)
			require.Equal(t, "alice", value.StringValue())
		})
	}
}

// TestSignedCursor_Rejected pins what a signer-configured ListCursor refuses,
// and with which error.
func TestSignedCursor_Rejected(t *testing.T) {
	t.Parallel()

	coll := newDetachedCollection(t)
	signer := newCursorSigner(t, 1)
	request := signedRequest(signer, "tenant-A")
	filter := bson.M{"status": "active"}
	sort := bson.D{{Key: "name", Value: 1}}

	unsigned, err := NewCursorWithMetadata(bson.NewObjectID().Hex(), sort, DefaultCursorIdField, filter, "alice")
	require.NoError(t, err)
	foreign, err := signer.Issue([]byte(`{}`), pagetoken.Bindings{Subject: "tenant-A/precision=1ms"})
	require.NoError(t, err)
	tampered := []byte(mintSignedCursor(t, coll, request...))
	tampered[len(tampered)/2] ^= 1

	tests := []struct {
		name   string
		target *mongo.Collection
		token  string
		opts   []ListCursorOption
		want   error
	}{
		{name: "tampered token", token: string(tampered), opts: request, want: ErrInvalidCursor},
		{name: "unsigned token", token: unsigned.String(), opts: request, want: ErrInvalidCursor},
		{name: "token of another list", token: foreign, opts: request, want: ErrInvalidCursor},
		{
			name:  "token signed with another key",
			token: mintSignedCursor(t, coll, signedRequest(newCursorSigner(t, 2), "tenant-A")...),
			opts:  request,
			want:  ErrInvalidCursor,
		},
		{
			name:  "expired token",
			token: mintSignedCursor(t, coll, request...),
			opts: signedRequest(newCursorSigner(t, 1, pagetoken.WithClock(func() time.Time {
				return time.Now().Add(2 * pagetoken.DefaultTTL)
			})), "tenant-A"),
			want: pagetoken.ErrExpiredToken,
		},
		{
			name:  "another subject",
			token: mintSignedCursor(t, coll, request...),
			opts:  signedRequest(signer, "tenant-B"),
			want:  ErrCursorSubjectMismatch,
		},
		{
			name:  "bound token without a subject",
			token: mintSignedCursor(t, coll, request...),
			opts:  signedRequest(signer, ""),
			want:  ErrCursorSubjectMismatch,
		},
		{
			name:  "unbound token under a subject",
			token: mintSignedCursor(t, coll, signedRequest(signer, "")...),
			opts:  request,
			want:  ErrCursorSubjectMismatch,
		},
		{
			name:   "another collection",
			target: coll.Database().Collection("other"),
			token:  mintSignedCursor(t, coll, request...),
			opts:   request,
			want:   ErrCursorCollectionMismatch,
		},
		{
			name:   "same name in another database",
			target: coll.Database().Client().Database("otherdb").Collection(coll.Name()),
			token:  mintSignedCursor(t, coll, request...),
			opts:   request,
			want:   ErrCursorCollectionMismatch,
		},
		{
			name:  "another filter",
			token: mintSignedCursor(t, coll, request...),
			opts:  append(request[:len(request):len(request)], WithListCursorFilter(bson.M{"status": "archived"})),
			want:  ErrCursorFilterMismatch,
		},
		{
			name:  "another sort",
			token: mintSignedCursor(t, coll, request...),
			opts:  append(request[:len(request):len(request)], WithListCursorSort("-name")),
			want:  ErrCursorChecksumMismatch,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			target := tc.target
			if target == nil {
				target = coll
			}
			_, err := ListCursor[bson.M](shortCtx(t), target, append(tc.opts[:len(tc.opts):len(tc.opts)], WithListCursorCursor(tc.token))...)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

// TestSignedCursor_ExpiredTokenIsInvalidCursor pins that a failed verification
// keeps both the package sentinel and the signer's own error in the chain.
func TestSignedCursor_ExpiredTokenIsInvalidCursor(t *testing.T) {
	t.Parallel()

	coll := newDetachedCollection(t)
	token := mintSignedCursor(t, coll, signedRequest(newCursorSigner(t, 1), "tenant-A")...)
	late := newCursorSigner(t, 1, pagetoken.WithClock(func() time.Time { return time.Now().Add(2 * pagetoken.DefaultTTL) }))

	_, err := resolveSignedCursor(t.Context(), late, token, cursorFilter{original: bson.M{"status": "active"}}, "tenant-A", cursorCollectionDigest(coll))
	require.ErrorIs(t, err, ErrInvalidCursor)
	require.ErrorIs(t, err, pagetoken.ErrExpiredToken)
	require.Equal(t, pagetoken.FailureExpired, pagetoken.Classify(err))
}

// TestSignedCursor_DirectCursorRejected pins that a *Cursor, which never
// passed the signer, is not accepted where one is configured.
func TestSignedCursor_DirectCursorRejected(t *testing.T) {
	t.Parallel()

	coll := newDetachedCollection(t)
	filter := bson.M{"status": "active"}
	sort := bson.D{{Key: "name", Value: 1}}
	direct, err := NewCursorWithMetadata(bson.NewObjectID().Hex(), sort, DefaultCursorIdField, filter, "alice")
	require.NoError(t, err)

	_, err = ListCursor[bson.M](shortCtx(t), coll,
		append(signedRequest(newCursorSigner(t, 1), "tenant-A"), WithListCursorCursor(direct))...)
	require.ErrorIs(t, err, ErrInvalidCursorFormat)
}

// TestListCursor_SubjectWithoutStorageOrSigner pins that a subject is never
// silently dropped: unsigned stateless mode cannot enforce it, so the call
// fails, on the first page as on a continuation.
func TestListCursor_SubjectWithoutStorageOrSigner(t *testing.T) {
	t.Parallel()

	coll := newDetachedCollection(t)
	filter := bson.M{"status": "active"}
	sort := bson.D{{Key: "name", Value: 1}}
	unsigned, err := NewCursorWithMetadata(bson.NewObjectID().Hex(), sort, DefaultCursorIdField, filter, "alice")
	require.NoError(t, err)

	tests := []struct {
		name string
		opts []ListCursorOption
	}{
		{name: "first page"},
		{name: "continuation", opts: []ListCursorOption{WithListCursorCursor(unsigned.String())}},
		{name: "nil signer", opts: []ListCursorOption{WithListCursorSigner(nil)}},
		{name: "typed nil signer", opts: []ListCursorOption{WithListCursorSigner((*pagetoken.Codec)(nil))}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ListCursor[bson.M](shortCtx(t), coll, append([]ListCursorOption{
				WithListCursorSubject("tenant-A"),
				WithListCursorFilter(filter),
				WithListCursorSort(sort),
			}, tc.opts...)...)
			require.ErrorIs(t, err, ErrCursorSubjectNotEnforced)
		})
	}
}

// TestListCursor_UnsignedStatelessWithoutSubject pins that stateless mode
// without a signer and without a subject resolves its tokens as before.
func TestListCursor_UnsignedStatelessWithoutSubject(t *testing.T) {
	t.Parallel()

	filter := bson.M{"status": "active"}
	sort := bson.D{{Key: "name", Value: 1}}
	unsigned, err := NewCursorWithMetadata(bson.NewObjectID().Hex(), sort, DefaultCursorIdField, filter, "alice")
	require.NoError(t, err)

	opts := newListCursorOptions(WithListCursorFilter(filter), WithListCursorSort(sort), WithListCursorCursor(unsigned.String()))
	require.NoError(t, resolveCursorOptions(t.Context(), opts, ""))
	require.Equal(t, unsigned.CursorId, opts.cursor.CursorId)
}

// TestListCursor_StorageTakesPrecedenceOverSigner pins that with both
// configured the cursor is stored: the token is a storage key, and a signed
// token is not accepted in its place.
func TestListCursor_StorageTakesPrecedenceOverSigner(t *testing.T) {
	t.Parallel()

	coll := newDetachedCollection(t)
	signer := newCursorSigner(t, 1)
	storage := newFakeCursorStorage()
	request := append(signedRequest(signer, "tenant-A"), WithListCursorStorage(storage))

	key := mintSignedCursor(t, coll, request...)
	require.True(t, isULID(key))
	require.Contains(t, storage.entries, key)

	signed := mintSignedCursor(t, coll, signedRequest(signer, "tenant-A")...)
	_, err := ListCursor[bson.M](shortCtx(t), coll, append(request[:len(request):len(request)], WithListCursorCursor(signed))...)
	require.ErrorIs(t, err, ErrInvalidCursorFormat)
}
