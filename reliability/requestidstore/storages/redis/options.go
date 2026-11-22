// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

import "time"

//go:generate go tool optgen generate

// DefaultKeyPrefix isolates request IDs from other Redis data.
const DefaultKeyPrefix = "requestid:"

// DefaultCleanupGrace retains expired entries for an extra hour before physical cleanup.
const DefaultCleanupGrace = time.Hour

type options struct {
	// WithCleanupGrace sets the retention margin for clock skew. Values below one millisecond are ignored.
	cleanupGrace time.Duration `optgen:"default=DefaultCleanupGrace,onfail=skip" optcheck:"min=time.Millisecond"`
	keyPrefix    string        `optgen:"default=DefaultKeyPrefix"`
}
