// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package nats_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers/servers/natstest"

	dlnats "github.com/altessa-s/go-atlas/coord/dlock/providers/nats"
)

// NewDLock wires a DLock over a NATS provider on the given bucket.
func TestNewDLock(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	nc, js := natstest.ConnectJetStream(t, ns)

	dl, err := dlnats.NewDLock(t.Context(), nc, "app-locks")
	require.NoError(t, err)
	require.NotNil(t, dl)

	ran := false
	require.NoError(t, dl.Synchronize(t.Context(), "resource", func(context.Context) error {
		ran = true
		return nil
	}))
	require.True(t, ran)

	_, err = js.KeyValue(t.Context(), "app-locks")
	require.NoError(t, err, "the lock bucket must be the one passed in")
}

// A provider that cannot be created is reported as an error, not a DLock.
func TestNewDLock_ProviderError(t *testing.T) {
	t.Parallel()

	ns := natstest.StartNATSServer(t)
	nc := natstest.ConnectNATS(t, ns)
	nc.Close()

	dl, err := dlnats.NewDLock(t.Context(), nc, "app-locks")
	require.Error(t, err)
	require.Nil(t, dl)
}
