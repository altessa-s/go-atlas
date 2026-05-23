// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

const (
	// DefaultKeyPrefix is the default prefix for Redis keys.
	DefaultKeyPrefix = "ratelimit:"
)

// options contains Redis provider configuration.
type options struct {
	// KeyPrefix sets the prefix for Redis keys.
	// Default is "ratelimit:".
	keyPrefix string `optgen:"default=DefaultKeyPrefix" optcheck:"nonempty"`
}
