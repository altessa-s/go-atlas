// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotencyit_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-aip/requestid"
	"github.com/altessa-s/go-aip/requestid/requestidtest"

	"github.com/altessa-s/go-atlas/tests/integration/internal/livebackend"

	requestnats "github.com/altessa-s/go-atlas/reliability/requestidstore/storages/nats"
)

func TestRequestIDNATS(t *testing.T) {
	t.Parallel()
	nc, err := nats.Connect(natsURL(), nats.Timeout(2*time.Second), nats.RetryOnFailedConnect(false))
	if err != nil {
		t.Skipf("NATS unreachable at %s: %v", natsURL(), err)
	}
	t.Cleanup(nc.Close)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	requestidtest.Run(t, func(t *testing.T) requestid.Store {
		bucket := livebackend.UniqueName(t, "requestid")
		s, err := requestnats.New(t.Context(), js, requestnats.WithBucket(bucket))
		require.NoError(t, err)
		t.Cleanup(func() {
			ctx := context.WithoutCancel(t.Context())
			require.NoError(t, s.Close(ctx))
			require.NoError(t, js.DeleteKeyValue(ctx, bucket))
		})
		return s
	})
}
