// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package noop

import (
	"context"
	"time"

	"github.com/altessa-s/go-atlas/data/uniq/providers"
)

// Provider implements the uniq.Provider interface with no actual storage.
// All operations are no-ops and return nil errors.
type Provider struct{}

var (
	_ providers.Provider = (*Provider)(nil)
	_ providers.Prober   = (*Provider)(nil)
)

// New creates a new NOP provider.
func New() *Provider { return &Provider{} }

// Add is a no-op implementation that always returns nil.
func (p *Provider) Add(_ context.Context, _ string) error { return nil }

// AddWithValue is a no-op implementation that always returns nil.
func (p *Provider) AddWithValue(_ context.Context, _ string, _ []byte) error { return nil }

// TryAdd is a no-op implementation that always reports success.
// Consistent with [Provider.Exist] always returning false: the noop
// provider has no storage, so no key is ever "already present". The
// ttl argument is ignored. Tests that need real CAS or TTL semantics
// must use a real backend.
func (p *Provider) TryAdd(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
}

// TryAddWithValue is the no-op counterpart to [Provider.TryAdd] for
// the value-bearing variant; both the value and ttl are discarded.
func (p *Provider) TryAddWithValue(_ context.Context, _ string, _ []byte, _ time.Duration) (bool, error) {
	return true, nil
}

// Exist is a no-op implementation that always returns false and nil error.
func (p *Provider) Exist(_ context.Context, _ string) (bool, error) { return false, nil }

// GetValue is a no-op implementation that always returns nil and nil error.
func (p *Provider) GetValue(_ context.Context, _ string) ([]byte, error) { return nil, nil }

// Remove is a no-op implementation that always returns nil.
func (p *Provider) Remove(_ context.Context, _ string) error { return nil }

// Clear is a no-op implementation that always returns nil.
func (p *Provider) Clear(_ context.Context) error { return nil }

// Probe implements [providers.Prober]. The noop provider has no
// underlying state to probe and always reports healthy.
func (p *Provider) Probe(_ context.Context) error { return nil }
