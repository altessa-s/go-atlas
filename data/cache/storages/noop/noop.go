// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package noop

import (
	"context"
	"time"

	"github.com/altessa-s/go-atlas/data/cache/storages"
)

// Storage is a no-operation cache provider that discards all writes.
// Get always returns ErrMissing. Useful for testing or disabling caching.
type Storage struct{}

// New creates a new no-op provider.
//
// Example:
//
//	p := noop.New()
func New() *Storage {
	return &Storage{}
}

// Save is a no-op that discards the value and returns nil.
func (p *Storage) Save(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return nil
}

// Get always returns [storages.ErrMissing] since no data is stored.
func (p *Storage) Get(_ context.Context, _ string) ([]byte, error) { return nil, storages.ErrMissing }

// Delete is a no-op and returns nil.
func (p *Storage) Delete(_ context.Context, _ string) error { return nil }

// Exists always returns false since no data is stored.
func (p *Storage) Exists(_ context.Context, _ string) (bool, error) { return false, nil }

// DeleteMany is a no-op and returns nil.
func (p *Storage) DeleteMany(_ context.Context, _ ...string) error { return nil }

var _ storages.Storage = (*Storage)(nil)
