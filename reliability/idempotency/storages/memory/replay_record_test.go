// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

import (
	"testing"
	"time"

	"github.com/altessa-s/go-atlas-core/time/clock"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages/storagetest"
)

func TestStorageContract(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, func(tb testing.TB) storages.Storage { return newStore(tb) })
}

func TestKeeperUsesMemoryClock(t *testing.T) {
	t.Parallel()
	fakeClock := clock.NewFake(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	store := newStore(t, WithClock(fakeClock))
	storagetest.KeeperUsesStorageClock(t, store)
}
