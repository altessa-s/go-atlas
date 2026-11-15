// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metadata

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/text/strings/intern"

	"google.golang.org/grpc"

	grpcmd "google.golang.org/grpc/metadata"
)

// TestNewCallMetadata_DoesNotInternUserAgent pins that the client-supplied
// user-agent never reaches the process-wide string interner, which unique
// values would otherwise fill.
//
// Serial: it measures the size of the process-wide GlobalInterner, which a
// parallel test could grow concurrently.
func TestNewCallMetadata_DoesNotInternUserAgent(t *testing.T) {
	info := &grpc.UnaryServerInfo{FullMethod: "/intern.Service/Method"}
	NewCallMetadata(t.Context(), info.FullMethod, info) // intern the method names once

	before := intern.Default().Size()
	for i := range 64 {
		ua := "agent/" + strconv.Itoa(i)
		ctx := grpcmd.NewIncomingContext(t.Context(), grpcmd.Pairs("user-agent", ua))
		meta := NewCallMetadata(ctx, info.FullMethod, info)
		require.Equal(t, ua, meta.ClientUserAgent)
	}

	require.Equal(t, before, intern.Default().Size())
}
