// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"context"
)

// PolicySource defines the interface for fetching OPA policies from various sources.
// Implementations can provide policies from the filesystem, HTTP bundles, or other sources.
type PolicySource interface {
	// Name returns the name identifier for this source.
	Name() string

	// Fetch retrieves the current policy bundle from the source.
	// Returns the bundle with all policy modules and optional data.
	Fetch(ctx context.Context) (*PolicyBundle, error)

	// Watch starts watching for policy changes and returns a channel that signals updates.
	// The channel receives a struct{} whenever policies may have changed.
	// Close the returned channel by calling Close() on the source.
	Watch(ctx context.Context) (<-chan struct{}, error)

	// Close releases any resources held by the source and stops watching.
	Close() error
}
