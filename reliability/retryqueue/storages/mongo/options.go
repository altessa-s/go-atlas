// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mongo

//go:generate go tool optgen generate

import "time"

// DefaultFlagField is the default document field holding [retryqueue.Item.Flagged].
const DefaultFlagField = "flagged"

// DefaultFlagReasonField is the default document field holding
// [retryqueue.Item.FlagReason].
const DefaultFlagReasonField = "flagReason"

type options struct {
	// flagField names the document field of the attention flag, so the store
	// can adopt a collection whose documents name it differently.
	flagField string `optval:"nonempty" optgen:"default=DefaultFlagField"`
	// flagReasonField names the document field of the flag's reason.
	flagReasonField string `optval:"nonempty" optgen:"default=DefaultFlagReasonField"`
	// resolvedRetention is how long a resolved item is kept before MongoDB's
	// TTL monitor purges it, rounded up to whole seconds; [New] rejects more
	// than math.MaxInt32 seconds. Zero, the default, creates no TTL index and
	// keeps resolved items until they are deleted.
	resolvedRetention time.Duration `optval:"positive"`
}
