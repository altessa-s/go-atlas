// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/data/saga"
	"github.com/altessa-s/go-atlas/data/saga/storages/storagetest"
)

func TestStorageContract(t *testing.T) {
	t.Parallel()
	// NATS KV keys admit only [-/_=.A-Za-z0-9], so IDs with spaces, quotes or
	// non-ASCII characters are rejected.
	storagetest.Run(t, func(tb testing.TB) saga.Storage { return newStore(tb) }, "ExactIdentity")
}
