// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/connectivity"

	observabilityconfig "github.com/altessa-s/go-atlas/config/observability"
)

// The gRPC client the factory builds for the OTLP adapter is closed when
// the adapter shuts down: the adapter leaves an injected connection open.
func TestCreateOTLPAdapter_ShutdownClosesGRPCClient(t *testing.T) {
	t.Parallel()
	cfg := observabilityconfig.DefaultTracing()
	cfg.Enabled = true
	cfg.Type = observabilityconfig.TracingTypeOTLP
	cfg.Adapters = &observabilityconfig.TracingAdapters{OTLP: &observabilityconfig.TracingOTLP{
		Endpoint: "127.0.0.1:4317",
		Protocol: observabilityconfig.OTLPProtocolGRPC,
		Insecure: true,
	}}

	adapter, err := createOTLPAdapter(t.Context(), cfg.Adapters)
	require.NoError(t, err)
	owned, ok := adapter.(*ownedConnAdapter)
	require.True(t, ok, "the gRPC adapter must own its client")
	conn, err := owned.client.GetConnection(t.Context())
	require.NoError(t, err)

	require.NoError(t, adapter.Shutdown(t.Context()))
	require.Equal(t, connectivity.Shutdown, conn.GetState())
}
