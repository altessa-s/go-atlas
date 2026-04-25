// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package noop

import (
	"context"

	"github.com/altessa-s/go-atlas/data/uniq/providers"
)

// Provider implements the uniq.Provider interface with no actual storage.
// All operations are no-ops and return nil errors.
type Provider struct{}

var _ providers.Provider = (*Provider)(nil)

// New creates a new NOP provider.
func New() *Provider { return &Provider{} }

// Add is a no-op implementation that always returns nil.
func (p *Provider) Add(_ context.Context, _ string) error { return nil }

// AddWithValue is a no-op implementation that always returns nil.
func (p *Provider) AddWithValue(_ context.Context, _ string, _ []byte) error { return nil }

// Exist is a no-op implementation that always returns false and nil error.
func (p *Provider) Exist(_ context.Context, _ string) (bool, error) { return false, nil }

// GetValue is a no-op implementation that always returns nil and nil error.
func (p *Provider) GetValue(_ context.Context, _ string) ([]byte, error) { return nil, nil }

// Remove is a no-op implementation that always returns nil.
func (p *Provider) Remove(_ context.Context, _ string) error { return nil }

// Clear is a no-op implementation that always returns nil.
func (p *Provider) Clear(_ context.Context) error { return nil }
