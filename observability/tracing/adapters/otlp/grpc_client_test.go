// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package otlp

import (
	"testing"

	"github.com/stretchr/testify/require"

	grpcclient "github.com/altessa-s/go-atlas/transport/grpc/client"
)

func TestWithGRPCClientOptions_Stores(t *testing.T) {
	t.Parallel()

	o := newOptions(WithGRPCClientOptions())
	require.Empty(t, o.grpcClientOptions)

	o = newOptions(
		WithGRPCClientOptions(grpcclient.WithoutProxy(), grpcclient.WithoutProxy()),
	)
	require.Len(t, o.grpcClientOptions, 2)
}

func TestNewGRPCClient_PropagatesExtraOptions(t *testing.T) {
	t.Parallel()

	cfg := &grpcClientConfig{
		endpoint:           "localhost:4317",
		extraClientOptions: []grpcclient.Option{grpcclient.WithoutProxy()},
	}
	c := newGRPCClient(cfg)
	require.Len(t, c.extraClientOptions, 1)
}
