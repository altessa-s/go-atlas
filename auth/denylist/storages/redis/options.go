// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

// DefaultKeyPrefix is the default prefix for revocation keys in Redis.
const DefaultKeyPrefix = "denylist:revoked:"

// options carries the tunable [Store] configuration.
type options struct {
	keyPrefix string `optgen:"default=DefaultKeyPrefix"`
}
