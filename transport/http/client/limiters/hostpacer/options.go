// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package hostpacer

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=options

import "time"

// DefaultMaxHosts is the number of tracked hosts above which [Pacer] drops
// the buckets that have refilled completely.
const DefaultMaxHosts = 1024

// options holds the tunables of a [Pacer]; the interval and burst are
// positional arguments of [New].
type options struct {
	// maxHosts is the number of tracked hosts above which a new host triggers
	// a sweep of the fully refilled buckets. Dropping such a bucket is
	// lossless: a fresh one starts full too.
	maxHosts int `optgen:"default=DefaultMaxHosts"`
	// now is the clock the sweep reads bucket levels at.
	now func() time.Time `optgen:"default=time.Now"`
}
