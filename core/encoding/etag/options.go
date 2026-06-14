// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package etag

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

import (
	"crypto/sha256"
	"hash"
)

// NewHashFunc constructs a fresh [hash.Hash] used to compute a strong content
// tag. Implementations must return a new instance on every call so concurrent
// generations never share state.
type NewHashFunc func() hash.Hash

// DefaultNewHash is the hash constructor used by a [Generator] when
// [WithNewHash] is not supplied. It uses SHA-256, whose collision resistance
// makes generated tags effectively unique per distinct content. Swap in a
// faster non-cryptographic hash via [WithNewHash] when that uniqueness margin
// is not required.
var DefaultNewHash NewHashFunc = sha256.New

// options holds the [Generator] configuration. Configure it through the
// generated [Option] values (the With* constructors).
type options struct {
	// newHash constructs the hash.Hash used for content-based strong tags.
	newHash NewHashFunc `optgen:"default=DefaultNewHash"`
}
