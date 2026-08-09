// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keyset

import (
	"context"
	"encoding/json"

	coreerrs "github.com/altessa-s/go-atlas/core/errors"
)

// Tokens is a typed layer over [KeySet] for consumers that page a data
// source: it owns the position's wire form, so integrating a new database
// stops at declaring a position struct and rendering the query bindings.
//
// P is what a position is for that source — the sort-key values of the last
// row a caller saw. It travels as JSON inside the token (or the server-side
// state), so its fields need json tags and nothing else:
//
//	type position struct {
//		CreatedAt int64  `json:"c"`
//		ID        string `json:"i"`
//	}
//
//	tokens := keyset.NewTokens[position](keys)
//	token, err := tokens.Issue(ctx, position{...}, bindings)
//	pos, err := tokens.Resolve(ctx, token, bindings)
//
// The same [Bindings] value serves both directions: Issue records its Sort,
// Filter, and Subject; Resolve checks them against the request being served.
type Tokens[P any] struct {
	keys *KeySet
}

// NewTokens wraps a KeySet, falling back to a self-contained one so a
// consumer built without an explicit configuration still pages.
func NewTokens[P any](keys *KeySet) *Tokens[P] {
	if keys == nil {
		keys = New()
	}

	return &Tokens[P]{keys: keys}
}

// Issue renders position as a token bound to the query described by
// bindings. See [KeySet.Issue] for the token forms and their guarantees.
func (t *Tokens[P]) Issue(ctx context.Context, position P, bindings Bindings) (Token, error) {
	payload, err := json.Marshal(position)
	if err != nil {
		return "", coreerrs.WrapOperation(err, "encode cursor position")
	}

	return t.keys.Issue(ctx, Cursor{
		Payload: payload,
		Sort:    bindings.Sort,
		Filter:  bindings.Filter,
	}, bindings.Subject)
}

// Resolve reads a token back into a position, after the KeySet has checked
// it against bindings. Failure modes are [KeySet.Resolve]'s; a payload that
// does not decode as P — a token minted for a different position shape —
// additionally fails with [ErrInvalidToken].
func (t *Tokens[P]) Resolve(ctx context.Context, token Token, bindings Bindings) (P, error) {
	var position P

	cursor, err := t.keys.Resolve(ctx, token, bindings)
	if err != nil {
		return position, err
	}

	if err := json.Unmarshal(cursor.Payload, &position); err != nil {
		return position, coreerrs.Wrap(ErrInvalidToken, "cursor position does not decode")
	}

	return position, nil
}

// Release drops a server-side cursor before the storage would; a no-op for
// self-contained tokens. See [KeySet.Release].
func (t *Tokens[P]) Release(ctx context.Context, token Token) error {
	return t.keys.Release(ctx, token)
}
