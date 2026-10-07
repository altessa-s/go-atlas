// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keyset

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import "time"

const (
	// DefaultTTL is how long an issued token stays valid.
	DefaultTTL = 24 * time.Hour

	// DefaultMaxClockSkew is how far in the future a token's issue time may
	// lie, to tolerate clock differences between replicas.
	DefaultMaxClockSkew = time.Minute

	// MinKeyLength is the shortest signing key [New] accepts, in bytes.
	MinKeyLength = 32

	// MaxPayloadLength bounds the payload of a token, in bytes.
	MaxPayloadLength = 1024
)

// Clock returns the current time. Tests replace it; production uses
// [time.Now].
type Clock func() time.Time

// defaultClock is the production wall clock.
var defaultClock Clock = time.Now

type options struct {
	// ttl is how long an issued token stays valid.
	ttl time.Duration `optgen:"default=DefaultTTL"`
	// expiryDisabled makes tokens valid indefinitely, ignoring ttl.
	expiryDisabled bool
	// maxClockSkew bounds how far in the future an issue time may lie.
	maxClockSkew time.Duration `optgen:"default=DefaultMaxClockSkew"`
	// previousKeys verify tokens signed before a key rotation; tokens are
	// always issued with the current key.
	previousKeys [][]byte `optgen:"append"`
	// clock is the time source.
	clock Clock `optgen:"default=defaultClock"`
}
