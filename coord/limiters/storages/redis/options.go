// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package redis

//go:generate go tool optgen generate

import "github.com/altessa-s/go-atlas-core/time/clock"

const (
	// DefaultKeyPrefix is the default prefix for Redis keys.
	DefaultKeyPrefix = "ratelimit:"
)

// options contains Redis provider configuration.
type options struct {
	// KeyPrefix sets the prefix for Redis keys.
	// Default is "ratelimit:".
	keyPrefix string `optgen:"default=DefaultKeyPrefix" optcheck:"nonempty"`

	// Clock supplies the request timestamps the scripts score and prune the
	// windows by, so a clock.Fake moves the windows in tests. Key expiry
	// (PEXPIRE, one period after the last request) still runs on the Redis
	// server clock; it only drops windows the fake time has not left yet
	// when the fake clock runs a period or more behind real time.
	clock clock.Clock `optgen:"default=clock.Real(),notnil"`
}
