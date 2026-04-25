// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leadelect

import (
	"time"
)

// Config holds leader election parameters.
type Config struct {
	// Key is the unique identifier for the election group.
	Key string

	// TTL is the lease duration before leadership expires.
	TTL time.Duration

	// NodeId is this instance's unique identifier.
	NodeId string
}
