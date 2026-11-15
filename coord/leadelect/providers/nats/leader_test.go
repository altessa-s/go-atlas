// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/coord/leadelect"
	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	lenats "github.com/altessa-s/go-atlas/coord/leadelect/providers/nats"
)

// NewLeader wires a Leader over a NATS provider built from the connection,
// passing the Leader options through.
func TestNewLeader(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	nc := natstest.ConnectNATS(t, ns)

	le, err := lenats.NewLeader(t.Context(), nc, "svc", "node-1", leadelect.WithTTL(5*time.Second))
	require.NoError(t, err)
	require.NotNil(t, le)
	require.False(t, le.IsLeader())
}

// A provider that cannot be created is reported as an error, not a Leader.
func TestNewLeader_ProviderError(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	nc := natstest.ConnectNATS(t, ns)
	nc.Close()

	le, err := lenats.NewLeader(t.Context(), nc, "svc", "node-1")
	require.Error(t, err)
	require.Nil(t, le)
}
