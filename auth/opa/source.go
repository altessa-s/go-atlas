// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package opa

import (
	"context"
)

// PolicySource defines the interface for fetching OPA policies from various sources.
// Implementations are passive data fetchers — the Manager handles all polling and
// change-detection scheduling.
type PolicySource interface {
	// Name returns the name identifier for this source.
	Name() string

	// Fetch retrieves the current policy bundle from the source.
	// Returns the bundle with all policy modules and optional data.
	Fetch(ctx context.Context) (*PolicyBundle, error)

	// Close releases any resources held by the source.
	Close() error
}
