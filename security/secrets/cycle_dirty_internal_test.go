// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets

import (
	"context"
	"errors"
	"iter"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

var errListFailed = errors.New("list failed")

// dirtyTestProvider is a minimal provider whose List result is configurable.
type dirtyTestProvider struct {
	mu      sync.Mutex
	values  map[string]string
	listErr error
}

func (p *dirtyTestProvider) Name() string { return "dirty-test" }

func (p *dirtyTestProvider) List(_ context.Context) ([]*Value[string], error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.listErr != nil {
		return nil, p.listErr
	}
	list := make([]*Value[string], 0, len(p.values))
	for k, v := range p.values {
		list = append(list, NewValue(k, v, nil, v))
	}
	return list, nil
}

func (p *dirtyTestProvider) Values(context.Context) iter.Seq2[*Value[string], error] {
	return func(func(*Value[string], error) bool) {}
}

func (p *dirtyTestProvider) Value(_ context.Context, key string) (*Value[string], error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.values[key]
	if !ok {
		return nil, ErrNotFound
	}
	return NewValue(key, v, nil, v), nil
}

func (p *dirtyTestProvider) Save(_ context.Context, key, value string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.values[key] = value
	return nil
}

func (p *dirtyTestProvider) Delete(_ context.Context, key string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.values, key)
	return nil
}

func (t *Manager[T]) cycleDirtyForTest() map[string]struct{} {
	t.cacheMu.Lock()
	defer t.cacheMu.Unlock()
	return t.cycleDirty
}

// TestManager_CycleDirtyOnlyDuringCycle checks that cache writes are only
// tracked while an update cycle runs, whatever its outcome, so the tracking
// cannot grow without bound.
func TestManager_CycleDirtyOnlyDuringCycle(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	provider := &dirtyTestProvider{values: map[string]string{"k": "v"}}
	mgr, err := New[string](provider, WithMaxRetries(1))
	require.NoError(t, err)

	require.NoError(t, mgr.Save(ctx, "a", "1"))
	require.NoError(t, mgr.Delete(ctx, "a"))
	_, err = mgr.Value(ctx, "k", true)
	require.NoError(t, err)
	require.Nil(t, mgr.cycleDirtyForTest(), "no tracking outside a cycle")

	provider.mu.Lock()
	provider.listErr = errListFailed
	provider.mu.Unlock()
	require.ErrorIs(t, mgr.RunUpdateCycle(ctx), errListFailed)
	require.Nil(t, mgr.cycleDirtyForTest(), "tracking ends after a failed cycle")

	provider.mu.Lock()
	provider.listErr = nil
	provider.values = map[string]string{}
	provider.mu.Unlock()
	require.NoError(t, mgr.RunUpdateCycle(ctx))
	require.Nil(t, mgr.cycleDirtyForTest(), "tracking ends after an empty cycle")

	require.NoError(t, mgr.Save(ctx, "b", "2"))
	require.NoError(t, mgr.RunUpdateCycle(ctx))
	require.Nil(t, mgr.cycleDirtyForTest(), "tracking ends after a successful cycle")
}
