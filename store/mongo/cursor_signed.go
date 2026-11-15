// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/altessa-s/go-aip/pagination/pagetoken"

	corekeys "github.com/altessa-s/go-atlas-core/encoding/keys"
	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// CursorSigner signs and verifies stateless cursor tokens, see
// WithListCursorSigner. A go-aip pagetoken codec implements it.
type CursorSigner interface {
	Issue(payload []byte, b pagetoken.Bindings) (string, error)
	Resolve(token string, b pagetoken.Bindings) ([]byte, error)
}

// signedCursorBindings separate list-cursor tokens from every other token
// issued with the same codec (keyset pages, audit queries): a token of
// another list does not resolve here, nor one of these there.
var signedCursorBindings = pagetoken.Bindings{Subject: "go-atlas/store/mongo.ListCursor"}

// signedCursorFields is Cursor without its methods, so the payload encodes
// the fields rather than the base64 string of Cursor.MarshalJSON.
type signedCursorFields Cursor

// signedCursor is the payload of a signed stateless token: the cursor and the
// request it is bound to.
type signedCursor struct {
	signedCursorFields

	// Collection is the digest of the collection the cursor was issued for,
	// see cursorCollectionDigest.
	Collection string `json:"col"`
	// Subject is the digest of the subject the cursor was issued to, empty
	// for an unbound cursor, see cursorSubjectDigest.
	Subject string `json:"sub,omitempty"`
}

// cursorSubjectDigest is the fixed-size form of subject a signed token
// carries. An empty subject stays empty: the cursor is unbound.
func cursorSubjectDigest(subject string) string {
	if subject == "" {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(corekeys.Digest([]byte(subject)))
}

// issueSignedCursor returns the signed stateless token of cursor, bound to
// collection and subject.
func issueSignedCursor(signer CursorSigner, cursor *Cursor, subject, collection string) (string, error) {
	payload, err := json.Marshal(signedCursor{
		signedCursorFields: signedCursorFields(*cursor),
		Collection:         collection,
		Subject:            cursorSubjectDigest(subject),
	})
	if err != nil {
		return "", coreerrs.WrapOperation(err, "encode cursor")
	}
	token, err := signer.Issue(payload, signedCursorBindings)
	if err != nil {
		return "", coreerrs.WrapOperation(err, "sign cursor")
	}
	return token, nil
}

// resolveSignedCursor verifies a signed stateless token and returns its
// cursor. The checks follow the stateful path: after the signature, the
// collection, the filter and the subject must equal the request's. An
// unsigned token does not verify and fails with [ErrInvalidCursor].
func resolveSignedCursor(signer CursorSigner, token string, filter bson.M, subject, collection string) (*Cursor, error) {
	if len(token) > MaxCursorTokenLength {
		return nil, coreerrs.Wrapf(ErrInvalidCursor, "token longer than %d bytes", MaxCursorTokenLength)
	}
	payload, err := signer.Resolve(token, signedCursorBindings)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCursor, err)
	}
	var signed signedCursor
	if err = json.Unmarshal(payload, &signed); err != nil {
		return nil, coreerrs.Wrapf(ErrInvalidCursor, "%v", err)
	}
	cursor := Cursor(signed.signedCursorFields)

	if signed.Collection != collection {
		return nil, ErrCursorCollectionMismatch
	}
	if err = cursor.ValidateFilter(filter); err != nil {
		return nil, err
	}
	if signed.Subject != cursorSubjectDigest(subject) {
		return nil, coreerrs.Wrap(ErrCursorSubjectMismatch, "signed cursor bound to a different principal")
	}
	return &cursor, nil
}
