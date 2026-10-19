// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/data/idempotency/storages"
	"github.com/altessa-s/go-atlas/data/idempotency/storages/storagetest"
	"github.com/altessa-s/go-atlas/internal/testhelpers"

	idempnats "github.com/altessa-s/go-atlas/data/idempotency/storages/nats"
)

func TestStorageContract(t *testing.T) {
	t.Parallel()
	storagetest.Run(t, func(tb testing.TB) storages.Storage {
		ns := testhelpers.StartNATSServer(tb)
		_, js := testhelpers.ConnectJetStream(tb, ns)
		// Subtest names carry a slash, which bucket names do not allow; each store has its own server.
		s, err := idempnats.New(js, idempnats.WithBucket("contract"))
		require.NoError(tb, err)
		return s
	})
}
