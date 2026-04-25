// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package noop

import (
	"context"
	"time"

	"github.com/altessa-s/go-atlas/data/cache/providers"
)

// Provider is a no-operation cache provider that discards all writes.
// Get always returns ErrMissing. Useful for testing or disabling caching.
type Provider struct{}

// New creates a new no-op provider.
//
// Example:
//
//	p := noop.New()
func New() *Provider {
	return &Provider{}
}

// Save is a no-op that discards the value and returns nil.
func (p *Provider) Save(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return nil
}

// Get always returns [providers.ErrMissing] since no data is stored.
func (p *Provider) Get(_ context.Context, _ string) ([]byte, error) { return nil, providers.ErrMissing }

// Delete is a no-op and returns nil.
func (p *Provider) Delete(_ context.Context, _ string) error { return nil }

// Exists always returns false since no data is stored.
func (p *Provider) Exists(_ context.Context, _ string) (bool, error) { return false, nil }

// DeleteMany is a no-op and returns nil.
func (p *Provider) DeleteMany(_ context.Context, _ ...string) error { return nil }

var _ providers.Provider = (*Provider)(nil)
