// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"google.golang.org/grpc/metadata"
)

// keyNamespace anchors derived keys to this package so that identical (seed, call)
// inputs produced elsewhere cannot collide with keys minted here.
const keyNamespace = "github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"

// RFC 4122 UUID v4 bit-layout constants — see §4.4. Named to keep [DeriveKey]
// readable without inline magic numbers.
const (
	uuidVersionByte   = 6    // byte index that carries the version nibble.
	uuidVariantByte   = 8    // byte index that carries the variant bits.
	uuidVersionMask   = 0x0f // clears the upper nibble of the version byte.
	uuidVersionV4     = 0x40 // sets the version nibble to 0100 (v4).
	uuidVariantMask   = 0x3f // clears the upper two bits of the variant byte.
	uuidVariantRFC    = 0x80 // sets the variant bits to 10 (RFC 4122).
	uuidBinarySize    = 16   // canonical UUID length in bytes.
	uuidCanonicalSize = 36   // canonical 8-4-4-4-12 hex-with-dashes length.
)

// DeriveKey returns a deterministic idempotency key for a single outbound mutating call.
// It is derived from seed — a stable identifier of the logical operation that stays
// constant across its retries — and call, a name identifying the specific downstream
// call. Typical call values are the gRPC full-method string (info.FullMethod, for
// example "/users.v1.UserService/Update"); any stable string works, but it must not
// drift between retries of the same logical operation.
//
// The same (seed, call) pair always yields the same key, so retries of one logical call
// are deduplicated by the server. Different call names yield different keys, so several
// downstream calls made while handling one operation do not collide — the server scopes
// idempotency per service, not per method.
//
// The result is a lowercase UUID v4. It is compatible with [DefaultKeyValidator]; a
// server using [WithKeyFormatValidator] to enforce a non-UUID format will reject keys
// produced by DeriveKey — in that case mint the key in the format the server expects
// and attach it with [WithKey].
//
// DeriveKey is safe for concurrent use.
//
// Example:
//
//	key := idempotency.DeriveKey(operationID, "/users.v1.UserService/Update")
func DeriveKey(seed, call string) string {
	sum := sha256.Sum256([]byte(keyNamespace + "\x00" + seed + "\x00" + call))

	// Lay the first uuidBinarySize bytes of the digest into the canonical UUID v4
	// shape per RFC 4122 §4.4. Cannot fail and needs no third-party UUID library.
	var u [uuidBinarySize]byte
	copy(u[:], sum[:uuidBinarySize])
	u[uuidVersionByte] = (u[uuidVersionByte] & uuidVersionMask) | uuidVersionV4
	u[uuidVariantByte] = (u[uuidVariantByte] & uuidVariantMask) | uuidVariantRFC

	var b [uuidCanonicalSize]byte
	hex.Encode(b[0:8], u[0:4])
	b[8] = '-'
	hex.Encode(b[9:13], u[4:6])
	b[13] = '-'
	hex.Encode(b[14:18], u[6:8])
	b[18] = '-'
	hex.Encode(b[19:23], u[8:10])
	b[23] = '-'
	hex.Encode(b[24:36], u[10:16])
	return string(b[:])
}

// WithKey returns a copy of ctx with key attached as outgoing gRPC metadata under
// [DefaultIdempotencyKeyHeader], preserving any existing outgoing metadata. Use it when
// the caller already holds an idempotency key; otherwise use [WithDerivedKey].
//
// WithKey is safe for concurrent use; it does not mutate ctx or its metadata.
func WithKey(ctx context.Context, key string) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		md = md.Copy()
	} else {
		md = metadata.New(nil)
	}

	md.Set(DefaultIdempotencyKeyHeader, key)
	return metadata.NewOutgoingContext(ctx, md)
}

// WithDerivedKey attaches a key produced by [DeriveKey] to ctx as outgoing gRPC metadata.
// It is shorthand for WithKey(ctx, DeriveKey(seed, call)).
//
// Example:
//
//	ctx = idempotency.WithDerivedKey(ctx, operationID, "users.UserService/Update")
//	resp, err := client.Update(ctx, req)
func WithDerivedKey(ctx context.Context, seed, call string) context.Context {
	return WithKey(ctx, DeriveKey(seed, call))
}
