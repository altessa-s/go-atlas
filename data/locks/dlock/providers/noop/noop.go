// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package noop

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-atlas/data/locks/dlock/providers"
)

// Provider is a no-operation implementation of the distributed lock provider.
// It always succeeds in acquiring locks but doesn't actually perform any locking.
type Provider struct {
	closed atomic.Bool
}

var (
	_ providers.Provider = (*Provider)(nil)
	_ providers.Prober   = (*Provider)(nil)
)

// New creates a new NOP provider.
func New() *Provider {
	return &Provider{}
}

// Lock implements the providers.Provider interface.
// It always returns a successful lock without actually performing any locking.
func (p *Provider) Lock(_ context.Context, key string) (providers.Lock, error) {
	if p.closed.Load() {
		return nil, errors.New("provider is closed")
	}

	return &Lock{
		info: &providers.LockInfo{
			Key:         key,
			Owner:       "nop",
			AcquiredAt:  time.Now(),
			LastRenewed: time.Now(),
			TTL:         0,
			IsStale:     false,
		},
	}, nil
}

// GetLockInfo implements the providers.Provider interface.
// It always returns successful lock info without actually checking any lock state.
func (p *Provider) GetLockInfo(_ context.Context, key string) (*providers.LockInfo, error) {
	return &providers.LockInfo{
		Key:         key,
		Owner:       "nop",
		AcquiredAt:  time.Now(),
		LastRenewed: time.Now(),
		TTL:         time.Hour,
		IsStale:     false,
	}, nil
}

// Lock represents a no-operation lock.
type Lock struct {
	info *providers.LockInfo
}

// GetLockInfo implements the providers.Lock interface.
func (l *Lock) GetLockInfo(ctx context.Context) (*providers.LockInfo, error) {
	return l.info, nil
}

// Release implements the providers.Lock interface.
// It does nothing as this is a no-operation lock but supports context.
func (l *Lock) Release(ctx context.Context) error {
	// Check if context is already canceled
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

// Close implements the providers.Provider interface.
// It marks the provider as closed to prevent new locks.
func (p *Provider) Close(_ context.Context) error {
	p.closed.Store(true)
	return nil
}

// Probe implements [providers.Prober]. The noop provider has no
// underlying state to probe, so it reports healthy until [Provider.Close].
func (p *Provider) Probe(_ context.Context) error {
	if p.closed.Load() {
		return errors.New("provider is closed")
	}
	return nil
}
