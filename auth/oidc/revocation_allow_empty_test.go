// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache/probfilter/stats"
)

// statsFilter is a rebuildable filter that reports its item count, like the
// Bloom and Cuckoo facades do.
type statsFilter struct {
	*rebuildableFilter
	statsErr error
}

func (f *statsFilter) Stats(context.Context) (*stats.FilterStats, error) {
	if f.statsErr != nil {
		return nil, f.statsErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return &stats.FilterStats{ItemCount: int64(len(f.data))}, nil
}

// revocationListFile writes a one-per-line revocation list and returns its path.
func revocationListFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "revoked.txt")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// With WithRevocationAllowEmpty an empty source is the new list: the filter is
// emptied and the sync counts as fresh.
func TestRevocationSync_AllowEmptyMakesEmptyResultAuthoritative(t *testing.T) {
	t.Parallel()

	for _, body := range [][]string{nil, {"", "  ", "\t"}} {
		idp := newTestIdP(t)
		idp.setRevoked("j1")
		p := newRevocationListProvider(t, idp, &URLRevocationLoader{URL: idp.srv.URL + "/revoked"},
			WithRevocationAllowEmpty())
		require.True(t, jtiRevoked(t, p, idp, "j1"))
		backdateRevocationSync(p, DefaultRevocationMaxStaleness)
		synced := p.lastRevocationSyncUnixNanos.Load()

		idp.setRevoked(body...)
		require.NoError(t, p.scheduledRevocationSync(t.Context()))
		require.False(t, jtiRevoked(t, p, idp, "j1"))
		require.Greater(t, p.lastRevocationSyncUnixNanos.Load(), synced, "an accepted empty sync is fresh")

		idp.setRevoked("j2")
		require.NoError(t, p.scheduledRevocationSync(t.Context()))
		require.True(t, jtiRevoked(t, p, idp, "j2"))
	}
}

// A storage passed in ready-made gets the provider's setting too, and the
// caller's storage is left as it was.
func TestRevocationAllowEmpty_AppliesToReadyMadeFilterStorage(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	idp.setRevoked("j1")
	storage := NewFilterRevocationStorage(newRebuildableFilter(), &URLRevocationLoader{URL: idp.srv.URL + "/revoked"}, nil)
	p := idp.newProvider(t, WithRevocationStorage(storage), WithRevocationItemType(RevocationItemTypeJTI),
		WithRevocationAllowEmpty())

	idp.setRevoked()
	require.NoError(t, p.scheduledRevocationSync(t.Context()))
	require.False(t, jtiRevoked(t, p, idp, "j1"))
	require.False(t, storage.(*filterRevocationStorage).allowEmpty)
}

// A storage goes by the filter's own item count: a replica that starts on a
// populated shared filter must not empty it on an empty source.
func TestRevocationSync_FirstSyncConsultsFilterItemCount(t *testing.T) {
	t.Parallel()

	errStats := errors.New("stats unavailable")
	tests := []struct {
		name       string
		held       []string
		statsErr   error
		allowEmpty bool
		wantErr    error
	}{
		{name: "populated filter keeps its set", held: []string{"j1"}, wantErr: ErrRevocationLoadFailed},
		{name: "unreadable item count keeps the set", held: []string{"j1"}, statsErr: errStats, wantErr: ErrRevocationLoadFailed},
		{name: "empty filter accepts an empty source"},
		{name: "allow empty overrides the item count", held: []string{"j1"}, allowEmpty: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			filter := &statsFilter{rebuildableFilter: newRebuildableFilter(), statsErr: tc.statsErr}
			for _, item := range tc.held {
				require.NoError(t, filter.Add(t.Context(), item))
			}
			storage := &filterRevocationStorage{
				filter:     filter,
				loader:     &FileRevocationLoader{Path: revocationListFile(t, "\n")},
				allowEmpty: tc.allowEmpty,
			}

			err := storage.Sync(t.Context())
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			for _, item := range tc.held {
				held, mightErr := filter.MightExist(t.Context(), item)
				require.NoError(t, mightErr)
				require.Equal(t, tc.wantErr != nil, held)
			}
		})
	}
}

// The filter's item count is read on every empty result, not only before the
// first own sync: entries that reached the filter after this storage synced an
// empty list — a peer's list, or items added with MarkRevoked — keep it.
func TestRevocationSync_ItemCountConsultedAfterOwnEmptySync(t *testing.T) {
	t.Parallel()

	filter := &statsFilter{rebuildableFilter: newRebuildableFilter()}
	storage := &filterRevocationStorage{filter: filter, loader: &FileRevocationLoader{Path: revocationListFile(t, "")}}

	require.NoError(t, storage.Sync(t.Context()))
	require.NoError(t, storage.Sync(t.Context()), "an empty filter keeps accepting an empty source")

	require.NoError(t, storage.MarkRevoked(t.Context(), "j1", 0))
	require.ErrorIs(t, storage.Sync(t.Context()), ErrRevocationLoadFailed)
	held, err := filter.MightExist(t.Context(), "j1")
	require.NoError(t, err)
	require.True(t, held)
}
