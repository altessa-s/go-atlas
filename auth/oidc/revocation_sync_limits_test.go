// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	coreio "github.com/altessa-s/go-atlas-core/io"
)

// newRevocationListProvider starts a provider keyed on the jti claim whose
// revocation filter is synced from loader.
func newRevocationListProvider(t *testing.T, idp *testIdP, loader *URLRevocationLoader, opts ...Option) *Provider {
	t.Helper()
	base := []Option{
		WithRevocationFilter(newRebuildableFilter()), WithRevocationLoader(loader),
		WithRevocationItemType(RevocationItemTypeJTI),
	}
	return idp.newProvider(t, append(base, opts...)...)
}

// jtiRevoked reports whether p rejects a token carrying jti as revoked.
func jtiRevoked(t *testing.T, p *Provider, idp *testIdP, jti string) bool {
	t.Helper()
	_, err := p.ValidateToken(t.Context(), idp.sign(t, idp.claims(map[string]any{"jti": jti})))
	if errors.Is(err, ErrTokenRevoked) {
		return true
	}
	require.NoError(t, err)
	return false
}

func TestScheduledRevocationSync_BoundedByTimeout(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	var hang atomic.Bool
	list := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hang.Load() {
			<-r.Context().Done()
			return
		}
		_, _ = w.Write([]byte("j1\n"))
	}))
	t.Cleanup(list.Close)
	p := newRevocationListProvider(t, idp, &URLRevocationLoader{URL: list.URL},
		WithRevocationSyncTimeout(100*time.Millisecond))

	hang.Store(true)
	require.ErrorIs(t, p.revocation.scheduledRevocationSync(t.Context()), context.DeadlineExceeded)
	require.True(t, jtiRevoked(t, p, idp, "j1"), "a timed-out sync keeps the current set")
}

func TestURLRevocationLoader_TrimsEntriesAndSkipsBlankLines(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	idp.setRevoked("  j1 \r", "", " \t ", "j2")
	p := newRevocationListProvider(t, idp, &URLRevocationLoader{URL: idp.srv.URL + "/revoked"})

	require.True(t, jtiRevoked(t, p, idp, "j1"))
	require.True(t, jtiRevoked(t, p, idp, "j2"))
	blank, err := p.revocation.storage.IsRevoked(t.Context(), "")
	require.NoError(t, err)
	require.False(t, blank, "a blank line is not an entry")
}

// A source that turns up empty must not un-revoke everything: the sync fails
// and the current set stays.
func TestRevocationSync_EmptyResultKeepsCurrentSet(t *testing.T) {
	t.Parallel()

	for _, body := range [][]string{nil, {"", "  ", "\t"}} {
		idp := newTestIdP(t)
		idp.setRevoked("j1")
		p := newRevocationListProvider(t, idp, &URLRevocationLoader{URL: idp.srv.URL + "/revoked"})
		synced := p.revocation.lastRevocationSync.Load()

		idp.setRevoked(body...)
		require.ErrorIs(t, p.revocation.scheduledRevocationSync(t.Context()), ErrRevocationLoadFailed)
		require.True(t, jtiRevoked(t, p, idp, "j1"))
		require.Equal(t, synced, p.revocation.lastRevocationSync.Load(), "a failed sync is not fresh")

		idp.setRevoked("j2")
		require.NoError(t, p.revocation.scheduledRevocationSync(t.Context()))
		require.False(t, jtiRevoked(t, p, idp, "j1"))
		require.True(t, jtiRevoked(t, p, idp, "j2"))
	}
}

// With nothing loaded yet an empty source is just an empty list.
func TestRevocationSync_EmptySourceAcceptedWhileSetIsEmpty(t *testing.T) {
	t.Parallel()

	idp := newTestIdP(t)
	p := newRevocationListProvider(t, idp, &URLRevocationLoader{URL: idp.srv.URL + "/revoked"})

	require.NoError(t, p.revocation.scheduledRevocationSync(t.Context()))
	require.False(t, jtiRevoked(t, p, idp, "j1"))
}

// An oversized entry or body fails the sync as a whole; a cut-off list is
// never applied.
func TestRevocationSync_OversizedListFailsWithoutPartialApply(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		maxBytes int64
		list     []string
		wantErr  error
	}{
		{name: "entry over the per-entry maximum", list: []string{"j2", strings.Repeat("a", MaxRevocationEntryBytes+1), "j3"}},
		{name: "body over the cap", maxBytes: 32, list: []string{"j2", strings.Repeat("b", 64), "j3"}, wantErr: coreio.ErrReadLimitExceeded},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			idp := newTestIdP(t)
			idp.setRevoked("j1")
			p := newRevocationListProvider(t, idp,
				&URLRevocationLoader{URL: idp.srv.URL + "/revoked", MaxResponseBytes: tc.maxBytes})

			idp.setRevoked(tc.list...)
			err := p.revocation.scheduledRevocationSync(t.Context())
			require.ErrorIs(t, err, ErrRevocationLoadFailed)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}
			require.True(t, jtiRevoked(t, p, idp, "j1"), "the current set stays")
			require.False(t, jtiRevoked(t, p, idp, "j2"), "entries read before the failure are not applied")
		})
	}
}

func TestFileRevocationLoader_TrimsEntriesAndSkipsBlankLines(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "revoked.txt")
	require.NoError(t, os.WriteFile(path, []byte(" j1 \r\n\n\t\nj2"), 0o600))

	var entries []string
	for v, err := range (&FileRevocationLoader{Path: path}).StreamValues(t.Context()) {
		require.NoError(t, err)
		entries = append(entries, v)
	}
	require.Equal(t, []string{"j1", "j2"}, entries)
}
