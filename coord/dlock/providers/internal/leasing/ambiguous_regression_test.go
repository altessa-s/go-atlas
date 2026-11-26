// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package leasing

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var errStoreUnavailable = errors.New("store unavailable")

type unavailableStore struct {
	Store
	releaseCalls int
}

func (*unavailableStore) Acquire(context.Context, string, string) (uint64, bool, error) {
	return 0, false, errStoreUnavailable
}

func (s *unavailableStore) Release(context.Context, string, string, uint64) error {
	s.releaseCalls++
	return errStoreUnavailable
}

func TestEngine_DropsExpiredAmbiguousAcquisitions(t *testing.T) {
	t.Parallel()
	store := &unavailableStore{}
	engine := New(store, Settings{
		Logger: slog.New(slog.DiscardHandler), TTL: time.Second, OperationsTimeout: time.Second,
	})
	now := time.Now()
	engine.Now = func() time.Time { return now }
	for range 1000 {
		_, err := engine.Lock(t.Context(), "k")
		require.ErrorIs(t, err, errStoreUnavailable)
		now = now.Add(engine.cfg.TTL)
	}
	require.Len(t, engine.active, 1)
	require.NoError(t, engine.Close(t.Context()))
	require.Equal(t, 1000, store.releaseCalls, "Close must skip expired acquisitions")
}
