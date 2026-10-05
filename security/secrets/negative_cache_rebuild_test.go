// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/probfilter"
	"github.com/altessa-s/go-atlas/data/probfilter/cuckoo"
	"github.com/altessa-s/go-atlas/security/secrets"

	cuckoomemory "github.com/altessa-s/go-atlas/data/probfilter/cuckoo/storages/memory"
	secretmemory "github.com/altessa-s/go-atlas/security/secrets/providers/memory"
)

// listHookProvider runs afterList once, right after List took its snapshot.
type listHookProvider struct {
	secrets.Provider[string]
	once      sync.Once
	afterList func()
}

func (p *listHookProvider) List(ctx context.Context) ([]*secrets.Value[string], error) {
	list, err := p.Provider.List(ctx)
	if err == nil && p.afterList != nil {
		p.once.Do(p.afterList)
	}
	return list, err
}

// TestManager_UpdateCycle_SaveAfterSnapshotSurvivesRebuild saves a key after
// the update cycle took its storage snapshot but before the rebuilt negative
// filter is committed: the key must stay visible through the filter, so
// lookups keep reaching the provider instead of answering ErrNotFound.
func TestManager_UpdateCycle_SaveAfterSnapshotSurvivesRebuild(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	inner, err := secretmemory.New(map[string]string{"existing-key": "v"})
	require.NoError(t, err)
	provider := &listHookProvider{Provider: inner}
	filter := cuckoo.New(cuckoomemory.New(cuckoomemory.WithCapacity(100)))

	mgr, err := secrets.New[string](provider, secrets.WithNegativeFilter(filter))
	require.NoError(t, err)

	provider.afterList = func() {
		require.NoError(t, mgr.Save(ctx, "saved-during-cycle", "new"))
	}
	require.NoError(t, mgr.RunUpdateCycle(ctx))

	ok, err := filter.MightExist(ctx, "saved-during-cycle")
	require.NoError(t, err)
	require.True(t, ok, "a key saved after the snapshot must survive the rebuild")
}

// deleteSpy counts Delete calls on a wrapped filter.
type deleteSpy struct {
	probfilter.DeletableFilter
	deletes atomic.Int32
}

func (s *deleteSpy) Delete(ctx context.Context, value string) (bool, error) {
	s.deletes.Add(1)
	return s.DeletableFilter.Delete(ctx, value)
}

// rebuildableDeleteSpy is a deleteSpy that is also rebuildable, like a Cuckoo
// filter.
type rebuildableDeleteSpy struct {
	*deleteSpy
	rebuildable probfilter.RebuildableFilter
}

func (s rebuildableDeleteSpy) Rebuild(ctx context.Context, l probfilter.DataLoader) error {
	return s.rebuildable.Rebuild(ctx, l)
}

func (s rebuildableDeleteSpy) LastRebuild() time.Time { return s.rebuildable.LastRebuild() }

// TestManager_Delete_RebuildableFilterNotFingerprintDeleted checks that a key
// deleted through the Manager is not fingerprint-deleted from a filter the
// update cycle rebuilds: such a delete could race a rebuild and remove a
// colliding live key. The rebuild drops the key instead.
func TestManager_Delete_RebuildableFilterNotFingerprintDeleted(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	provider, err := secretmemory.New(map[string]string{"key-a": "1", "key-b": "2"})
	require.NoError(t, err)
	inner := cuckoo.New(cuckoomemory.New(cuckoomemory.WithCapacity(100)))
	spy := rebuildableDeleteSpy{deleteSpy: &deleteSpy{DeletableFilter: inner}, rebuildable: inner}

	mgr, err := secrets.New[string](provider, secrets.WithNegativeFilter(spy))
	require.NoError(t, err)
	require.NoError(t, mgr.RunUpdateCycle(ctx))

	require.NoError(t, mgr.Delete(ctx, "key-a"))
	require.Zero(t, spy.deletes.Load(), "a rebuildable filter must not be fingerprint-deleted")

	require.NoError(t, mgr.RunUpdateCycle(ctx))
	ok, err := inner.MightExist(ctx, "key-b")
	require.NoError(t, err)
	require.True(t, ok)
}

// TestManager_Delete_NonRebuildableFilterStillDeleted keeps the old behavior
// for deletable filters the update cycle cannot rebuild.
func TestManager_Delete_NonRebuildableFilterStillDeleted(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	provider, err := secretmemory.New(map[string]string{"key-a": "1"})
	require.NoError(t, err)
	spy := &deleteSpy{DeletableFilter: cuckoo.New(cuckoomemory.New(cuckoomemory.WithCapacity(100)))}
	require.NoError(t, spy.Add(ctx, "key-a"))

	mgr, err := secrets.New[string](provider, secrets.WithNegativeFilter(spy))
	require.NoError(t, err)

	require.NoError(t, mgr.Delete(ctx, "key-a"))
	require.Equal(t, int32(1), spy.deletes.Load())
}
