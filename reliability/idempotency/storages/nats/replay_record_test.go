// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages"
	"github.com/altessa-s/go-atlas/reliability/idempotency/storages/storagetest"

	idempnats "github.com/altessa-s/go-atlas/reliability/idempotency/storages/nats"
)

func TestStorageContract(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, func(tb testing.TB) storages.Storage {
		ns := natstest.StartNATSServer(tb)
		_, js := natstest.ConnectJetStream(tb, ns)
		// Subtest names carry a slash, which bucket names do not allow; each store has its own server.
		s, err := idempnats.New(js, idempnats.WithBucket("contract"))
		require.NoError(tb, err)
		return s
	})
}

func TestKeeperLeaseExpiresAndFencesHolder(t *testing.T) {
	t.Parallel()
	storagetest.KeeperUsesLockLease(t, setupStorage(t))
}
