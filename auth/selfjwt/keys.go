// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package selfjwt

import (
	"context"

	"github.com/altessa-s/go-atlas/auth/jwt"
)

// KeyProvider supplies both capabilities for [New].
type KeyProvider interface {
	SigningKeyProvider
	VerificationKeyProvider
}

// SigningKeyProvider supplies the signing material needed by [Minter].
type SigningKeyProvider interface {
	// SigningKey returns the subject's current signing material. Implementations
	// return [ErrSubjectUnknown] when the subject has no keys.
	SigningKey(ctx context.Context, subject string) (jwt.SigningKey, error)
}

// VerificationKeyProvider resolves verification keys for [Verifier].
type VerificationKeyProvider interface {
	// VerificationKey resolves a token's kid to the subject's public key,
	// accepting the current signing key and any retired-but-valid key still in
	// the rotation overlap window. Implementations return [ErrSubjectUnknown]
	// when the subject is unknown and [ErrKeyRotated] when the subject is known
	// but the kid is not.
	VerificationKey(ctx context.Context, subject, kid string) (jwt.VerificationKey, error)
}
