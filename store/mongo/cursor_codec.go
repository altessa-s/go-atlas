// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"

	"github.com/oklog/ulid/v2"
	"go.mongodb.org/mongo-driver/v2/bson"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
)

// cursorCodec issues and resolves only the format permitted by this listing.
type cursorCodec interface {
	issue(ctx context.Context, opts *listCursorOptions, cursorID string, sortValue any, collection string) (string, error)
	resolve(ctx context.Context, opts *listCursorOptions, collection string) (*Cursor, error)
}

func (o *listCursorOptions) nextCursorToken(ctx context.Context, id string, sortValue any, collection string) (string, error) {
	return o.codec.issue(ctx, o, id, sortValue, collection)
}

func selectCursorCodec(o *listCursorOptions) cursorCodec {
	switch {
	case o.storage != nil:
		return storedCursorCodec{}
	case o.signer != nil:
		return signedCursorCodec{}
	default:
		return unsignedCursorCodec{}
	}
}

type unsignedCursorCodec struct{}
type signedCursorCodec struct{}
type storedCursorCodec struct{}

func (o *listCursorOptions) statelessCursor(id string, sortValue any) (*Cursor, error) {
	c, err := newCursorWithMetadata(id, o.sort, o.cursorIdField, o.binding(), sortValue)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "create cursor")
	}
	return c, nil
}

func (codec unsignedCursorCodec) issue(_ context.Context, o *listCursorOptions, id string, sortValue any, _ string) (string, error) {
	c, err := o.statelessCursor(id, sortValue)
	if err != nil {
		return "", err
	}
	return codec.encode(c), nil
}

func (signedCursorCodec) issue(_ context.Context, o *listCursorOptions, id string, sortValue any, collection string) (string, error) {
	c, err := o.statelessCursor(id, sortValue)
	if err != nil {
		return "", err
	}
	return issueSignedCursor(o.signer, c, o.subject, collection)
}

func (storedCursorCodec) issue(ctx context.Context, o *listCursorOptions, id string, sortValue any, collection string) (string, error) {
	return storeCursor(ctx, id, o.sort, o.cursorIdField, o.binding(), o.storage, sortValue, o.subject, collection)
}

func (unsignedCursorCodec) resolve(ctx context.Context, o *listCursorOptions, _ string) (*Cursor, error) {
	if o.subject != "" {
		return nil, ErrCursorSubjectNotEnforced
	}
	c := o.cursor
	if o.cursorToken != "" {
		if isULID(o.cursorToken) {
			return nil, coreerrs.Wrap(ErrStorageRequired, "received ULID cursor but no storage configured")
		}
		var err error
		c, err = ParseCursor(o.cursorToken)
		if err != nil {
			return nil, err
		}
		if err := c.validateFilter(ctx, o.binding()); err != nil {
			return nil, err
		}
	}
	return validatePageCursor(ctx, o, c, o.cursorToken == "")
}

func (signedCursorCodec) resolve(ctx context.Context, o *listCursorOptions, collection string) (*Cursor, error) {
	if !o.cursor.IsZero() {
		return nil, coreerrs.Wrap(ErrInvalidCursorFormat, "cursor signer is configured: only signed tokens are accepted")
	}
	if o.cursorToken == "" {
		return o.cursor, nil
	}
	c, err := resolveSignedCursor(ctx, o.signer, o.cursorToken, o.binding(), o.subject, collection)
	if err != nil {
		return nil, err
	}
	return validatePageCursor(ctx, o, c, false)
}

func validatePageCursor(ctx context.Context, o *listCursorOptions, c *Cursor, direct bool) (*Cursor, error) {
	if !c.IsZero() {
		if err := validateStatelessCursor(ctx, c, o.sort, o.cursorIdField, o.binding(), direct); err != nil {
			return nil, coreerrs.WrapOperation(err, "validate cursor")
		}
	}
	return c, nil
}

func (storedCursorCodec) resolve(ctx context.Context, o *listCursorOptions, collection string) (*Cursor, error) {
	if !o.cursor.IsZero() {
		return nil, coreerrs.Wrap(ErrInvalidCursorFormat, "cursor storage is configured: only server-issued storage keys are accepted")
	}
	if o.cursorToken == "" {
		return o.cursor, nil
	}
	c, err := loadStoredCursor(ctx, o.cursorToken, o.storage, o.binding(), o.subject, collection)
	if err != nil {
		return nil, err
	}
	if !c.IsZero() {
		if err := c.ValidateChecksum(o.sort, o.cursorIdField); err != nil {
			return nil, coreerrs.WrapOperation(err, "validate cursor")
		}
	}
	return c, nil
}

func loadStoredCursor(ctx context.Context, token string, storage Storage, filter cursorFilter, subject, collection string) (*Cursor, error) {
	if !isULID(token) {
		return nil, coreerrs.Wrap(ErrInvalidCursorFormat, "cursor storage is configured: only server-issued storage keys are accepted")
	}

	// Load metadata from storage
	metadata, err := storage.Load(ctx, token)
	if err != nil {
		return nil, coreerrs.WrapOperation(err, "load cursor from storage")
	}

	if metadata == nil {
		return nil, ErrCursorNotFound
	}

	// Validate the cursor was issued for this collection: storages are
	// shared between endpoints, and a filter hash alone does not tell them apart.
	if validationErr := metadata.ValidateCollection(collection); validationErr != nil {
		return nil, validationErr
	}

	// Validate filter hasn't changed
	if validationErr := metadata.validateFilter(ctx, filter); validationErr != nil {
		return nil, validationErr
	}

	// Validate the cursor belongs to the requesting principal: the stored
	// subject must equal the request's, an unbound cursor included.
	if validationErr := metadata.ValidateSubject(subject); validationErr != nil {
		return nil, validationErr
	}

	// Convert metadata to Cursor
	cursor, convErr := metadata.ToCursor()
	if convErr != nil {
		return nil, coreerrs.WrapOperation(convErr, "convert metadata to cursor")
	}

	return cursor, nil
}

func storeCursor(
	ctx context.Context,
	cursorId string,
	sort bson.D,
	cursorIdField string,
	filter cursorFilter,
	storage Storage,
	sortValue any,
	subject string,
	collection string,
) (string, error) {
	// Server-side storage mode: create metadata and store
	metadata, err := newCursorMetadata(cursorId, sort, cursorIdField, filter, sortValue)
	if err != nil {
		return "", coreerrs.WrapOperation(err, "create cursor metadata")
	}

	// Bind the cursor to the requesting principal so it can't be replayed by
	// another. Empty subject leaves the cursor unbound (the default).
	metadata.Subject = subject
	metadata.Collection = collection

	// The key is a bearer handle: its entropy comes from crypto/rand, not from ulid.Make's monotonic math/rand source,
	// whose next value follows from the previous key.
	id, err := ulid.New(ulid.Now(), rand.Reader)
	if err != nil {
		return "", coreerrs.WrapOperation(err, "generate cursor key")
	}
	key := id.String()

	// Store with generated key
	if err := storage.Store(ctx, key, metadata); err != nil {
		return "", coreerrs.WrapOperation(err, "store cursor")
	}

	return key, nil
}

func (unsignedCursorCodec) encode(c *Cursor) string {
	if c == nil {
		return ""
	}

	// JSON encode using anonymous struct to avoid infinite recursion
	// with MarshalJSON method
	type cursorAlias Cursor
	jsonBytes, err := json.Marshal((*cursorAlias)(c))
	if err != nil {
		// Defensive: should never happen with a simple struct, but don't crash the process.
		return ""
	}

	// Base64 encode
	return base64.URLEncoding.EncodeToString(jsonBytes)
}

func (unsignedCursorCodec) decode(encoded string) (*Cursor, error) {
	if encoded == "" {
		return nil, coreerrs.Wrap(ErrInvalidCursor, "empty string")
	}
	if len(encoded) > MaxCursorTokenLength {
		return nil, coreerrs.Wrapf(ErrInvalidCursor, "token longer than %d bytes", MaxCursorTokenLength)
	}

	// Base64 decode
	jsonBytes, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, coreerrs.Wrapf(ErrInvalidCursor, "%v", err)
	}

	// JSON decode using alias to avoid calling UnmarshalJSON
	type cursorAlias Cursor
	var c cursorAlias
	if err = json.Unmarshal(jsonBytes, &c); err != nil {
		return nil, coreerrs.Wrapf(ErrInvalidCursor, "%v", err)
	}

	// Validation
	if c.CursorId == "" {
		return nil, coreerrs.Wrap(ErrInvalidCursor, "cursor_id is required")
	}

	// Validate cursor ID format
	if !isValidCursorId(c.CursorId) {
		return nil, coreerrs.Wrap(ErrInvalidCursor, "invalid cursor: cursor_id must be a MongoDB ObjectID (24 hex characters)")
	}

	return (*Cursor)(&c), nil
}
