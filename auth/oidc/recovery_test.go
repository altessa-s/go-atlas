// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

type recoveryStorage struct {
	syncStorage
	entered chan context.Context
	release chan struct{}
}

func (s *recoveryStorage) IsRevoked(context.Context, string) (bool, error) {
	return false, ErrRevocationReadinessLost
}
func (s *recoveryStorage) Sync(ctx context.Context) error {
	s.syncs.Add(1)
	s.entered <- ctx
	<-ctx.Done()
	<-s.release
	return ctx.Err()
}

func TestReadinessRecoveryLifetime(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		storage := &recoveryStorage{entered: make(chan context.Context, 8), release: make(chan struct{})}
		release := sync.OnceFunc(func() { close(storage.release) })
		defer release()
		p := newLocalRevocationProvider(t, "token", storage)
		p.revocation.syncTimeout = time.Minute
		p.backgroundCtx, p.cancelBackgroundCtx = context.WithCancel(t.Context())
		p.revocation.backgroundCtx = p.backgroundCtx
		defer p.cancelBackgroundCtx()
		err := p.revocation.checkLocalRevocation(t.Context(), "item", nil, nil)
		require.ErrorIs(t, err, ErrRevocationReadinessLost)
		synctest.Wait()
		require.Len(t, storage.entered, 1)
		ctx := <-storage.entered
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.Equal(t, time.Minute, time.Until(deadline))
		go func() { _ = p.revocation.scheduledRevocationSync(p.backgroundCtx) }()
		_ = p.revocation.checkLocalRevocation(t.Context(), "item", nil, nil)
		synctest.Wait()
		require.EqualValues(t, 1, storage.syncs.Load())
		closed := make(chan struct{})
		go func() { p.Close(); close(closed) }()
		<-ctx.Done()
		require.ErrorIs(t, ctx.Err(), context.Canceled)
		select {
		case <-closed:
			require.FailNow(t, "Close returned during Sync")
		default:
		}
		release()
		<-closed
		_ = p.revocation.checkLocalRevocation(t.Context(), "item", nil, nil)
		synctest.Wait()
		require.EqualValues(t, 1, storage.syncs.Load())
	})
}
