// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

//go:generate go tool optgen generate

// DefaultKeyPrefix isolates request IDs from other Redis data.
const DefaultKeyPrefix = "requestid:"

type options struct {
	keyPrefix string `optgen:"default=DefaultKeyPrefix"`
}
