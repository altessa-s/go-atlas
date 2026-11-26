// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

//go:generate go tool optgen generate

package memory

// DefaultMaxEvents is the default maximum number of audit events retained in memory.
const DefaultMaxEvents = 10_000

// options configures [Storage].
type options struct {
	// WithMaxEvents sets the ring buffer capacity. Storing beyond capacity
	// evicts the oldest events. Values below 1 are ignored.
	maxEvents int `optcheck:"min=1" optgen:"default=DefaultMaxEvents,onfail=skip"`
}
