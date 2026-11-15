// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package serviceinfo_test

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/runtime/appinfo"
	"github.com/altessa-s/go-atlas/transport/grpc/handlers/serviceinfo"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	serviceinfov1 "github.com/altessa-s/proto-gen-go/io/altessa/serviceinfo/v1"
)

func TestRegister_ServesOverGRPC(t *testing.T) {
	t.Parallel()

	h := serviceinfo.New(
		serviceinfo.WithServiceName("billing"),
		serviceinfo.WithServiceDescription("  Billing service  "),
		serviceinfo.WithServiceID("node-1"),
		serviceinfo.WithLeader(func(context.Context) (bool, string) { return true, "" }),
	)

	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	h.Register(gs, nil)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	resp, err := serviceinfov1.NewServiceInfoServiceClient(conn).GetServiceInfo(t.Context(), &serviceinfov1.GetServiceInfoRequest{})
	require.NoError(t, err)
	info := resp.GetServiceInfo()
	require.Equal(t, "billing", info.GetServiceName())
	require.Equal(t, "Billing service", info.GetServiceDescription(), "description is trimmed")
	require.Equal(t, "node-1", info.GetServiceId())
	require.True(t, info.GetLeader())
	require.Equal(t, "node-1", info.GetLeaderId(), "unknown leader ID falls back to the service ID")
}

func TestStringOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		opts     []serviceinfo.Option
		wantName string
		wantDesc *string
		wantID   *string
	}{
		{
			name:     "pointer values are applied and trimmed",
			opts:     []serviceinfo.Option{serviceinfo.WithServiceName(new(" api ")), serviceinfo.WithServiceDescription(new("desc")), serviceinfo.WithServiceID(new(" id-7 "))},
			wantName: "api", wantDesc: new("desc"), wantID: new("id-7"),
		},
		{
			name:     "nil pointers keep defaults",
			opts:     []serviceinfo.Option{serviceinfo.WithServiceName((*string)(nil)), serviceinfo.WithServiceDescription((*string)(nil)), serviceinfo.WithServiceID((*string)(nil))},
			wantName: appinfo.Name,
		},
		{
			name:     "blank strings keep defaults",
			opts:     []serviceinfo.Option{serviceinfo.WithServiceName("  "), serviceinfo.WithServiceDescription(" "), serviceinfo.WithServiceID("")},
			wantName: appinfo.Name,
		},
		{
			name:     "blank pointers keep defaults",
			opts:     []serviceinfo.Option{serviceinfo.WithServiceName(new(" ")), serviceinfo.WithServiceDescription(new("")), serviceinfo.WithServiceID(new("  "))},
			wantName: appinfo.Name,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := serviceinfo.New(tc.opts...).Snapshot(t.Context())
			require.Equal(t, tc.wantName, out.GetServiceName())
			require.Equal(t, tc.wantDesc, out.ServiceDescription)
			require.Equal(t, tc.wantID, out.ServiceId)
		})
	}
}

func TestWithLeaderProvider_NilIsIgnored(t *testing.T) {
	t.Parallel()

	out := serviceinfo.New(
		serviceinfo.WithServiceID("node-a"),
		serviceinfo.WithLeaderProvider(nil),
	).Snapshot(t.Context())
	require.False(t, out.GetLeader())
	require.Nil(t, out.LeaderId, "nil provider leaves leader fields unset")
}
