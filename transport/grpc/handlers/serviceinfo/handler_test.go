// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package serviceinfo_test

import (
	"context"
	"errors"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/runtime/appinfo"
	"github.com/altessa-s/go-atlas/transport/grpc/handlers/serviceinfo"

	"google.golang.org/protobuf/types/known/emptypb"
)

type stubLeader struct {
	isLeader bool
	leaderID string
	err      error
	calls    int
}

func (s *stubLeader) IsLeader() bool { return s.isLeader }
func (s *stubLeader) LeaderId(_ context.Context) (string, error) {
	s.calls++
	return s.leaderID, s.err
}

func TestNew_DefaultsFromAppinfo(t *testing.T) {
	t.Parallel()

	h := serviceinfo.New()
	out, err := h.Get(t.Context(), &emptypb.Empty{})
	require.NoError(t, err)
	require.NotNil(t, out)

	require.Equal(t, appinfo.Name, out.GetServiceName())
	require.Equal(t, appinfo.Version, out.GetFullVersion())
	require.False(t, out.GetLeader())
	require.Nil(t, out.LeaderId, "no leader provider configured -> leader_id must be unset")

	md := out.GetMetadata()
	require.Equal(t, runtime.Version(), md["go_version"])
	require.Equal(t, runtime.GOOS, md["go_os"])
	require.Equal(t, runtime.GOARCH, md["go_arch"])

	require.NotEmpty(t, out.GetStartTime())
	require.NotNil(t, out.Uptime)
}

func TestNew_OptionalFieldsOmittedWhenEmpty(t *testing.T) {
	t.Parallel()

	out, err := serviceinfo.New().Get(t.Context(), &emptypb.Empty{})
	require.NoError(t, err)

	require.Nil(t, out.ServiceDescription)
	require.Nil(t, out.ServiceId)
	// Note: ProtoVersion field was removed from serviceinfov1.ServiceInfo
	// If proto version info is needed, it should be added through WithExtraMetadata
}

func TestGet_LeaderProviderIsLeader(t *testing.T) {
	t.Parallel()

	leader := &stubLeader{isLeader: true}
	h := serviceinfo.New(
		serviceinfo.WithServiceID("node-a"),
		serviceinfo.WithLeaderProvider(leader),
	)

	out, err := h.Get(t.Context(), &emptypb.Empty{})
	require.NoError(t, err)
	require.True(t, out.GetLeader())
	require.Equal(t, "node-a", out.GetLeaderId(), "leader uses own serviceID without consulting LeaderId(ctx)")
	require.Equal(t, 0, leader.calls, "LeaderId(ctx) must not be called when this instance is leader")
}

func TestGet_LeaderProviderNotLeader(t *testing.T) {
	t.Parallel()

	leader := &stubLeader{isLeader: false, leaderID: "node-b"}
	h := serviceinfo.New(
		serviceinfo.WithServiceID("node-a"),
		serviceinfo.WithLeaderProvider(leader),
	)

	out, err := h.Get(t.Context(), &emptypb.Empty{})
	require.NoError(t, err)
	require.False(t, out.GetLeader())
	require.Equal(t, "node-b", out.GetLeaderId())
	require.Equal(t, 1, leader.calls)
}

func TestGet_LeaderProviderErrorFallsBackToServiceID(t *testing.T) {
	t.Parallel()

	leader := &stubLeader{isLeader: false, err: errors.New("kv timeout")}
	h := serviceinfo.New(
		serviceinfo.WithServiceID("node-a"),
		serviceinfo.WithLeaderProvider(leader),
	)

	out, err := h.Get(t.Context(), &emptypb.Empty{})
	require.NoError(t, err)
	require.Equal(t, "node-a", out.GetLeaderId(), "error from LeaderId(ctx) -> fall back to own serviceID")
}

func TestGet_LeaderProviderEmptyLeaderIDFallsBackToServiceID(t *testing.T) {
	t.Parallel()

	leader := &stubLeader{isLeader: false, leaderID: ""}
	h := serviceinfo.New(
		serviceinfo.WithServiceID("node-a"),
		serviceinfo.WithLeaderProvider(leader),
	)

	out, err := h.Get(t.Context(), &emptypb.Empty{})
	require.NoError(t, err)
	require.Equal(t, "node-a", out.GetLeaderId())
}

func TestGet_LeaderFunc(t *testing.T) {
	t.Parallel()

	var ctxSeen context.Context
	h := serviceinfo.New(
		serviceinfo.WithServiceID("node-a"),
		serviceinfo.WithLeader(func(ctx context.Context) (bool, string) {
			ctxSeen = ctx
			return false, "node-c"
		}),
	)

	out, err := h.Get(t.Context(), &emptypb.Empty{})
	require.NoError(t, err)
	require.False(t, out.GetLeader())
	require.Equal(t, "node-c", out.GetLeaderId())
	require.NotNil(t, ctxSeen, "request context must reach the leader func")
}

func TestWithLeader_NilFnIsNoOp(t *testing.T) {
	t.Parallel()

	h := serviceinfo.New(serviceinfo.WithLeader(nil))
	out, err := h.Get(t.Context(), &emptypb.Empty{})
	require.NoError(t, err)
	require.False(t, out.GetLeader())
	require.Nil(t, out.LeaderId)
}

func TestWithLeader_LastCallWins(t *testing.T) {
	t.Parallel()

	provider := &stubLeader{isLeader: true}
	h := serviceinfo.New(
		serviceinfo.WithServiceID("node-a"),
		serviceinfo.WithLeaderProvider(provider),
		serviceinfo.WithLeader(func(_ context.Context) (bool, string) { return false, "func-leader" }),
	)

	out, err := h.Get(t.Context(), &emptypb.Empty{})
	require.NoError(t, err)
	require.False(t, out.GetLeader(), "WithLeader applied last must win over WithLeaderProvider")
	require.Equal(t, "func-leader", out.GetLeaderId())
}

func TestWithMetadata_OverridesBuiltins(t *testing.T) {
	t.Parallel()

	h := serviceinfo.New(serviceinfo.WithExtraMetadata(map[string]string{
		"go_version": "custom-go",
		"region":     "eu-west-1",
	}))

	out, err := h.Get(t.Context(), &emptypb.Empty{})
	require.NoError(t, err)
	md := out.GetMetadata()
	require.Equal(t, "custom-go", md["go_version"], "extra metadata wins on conflict")
	require.Equal(t, "eu-west-1", md["region"])
	require.Equal(t, runtime.GOOS, md["go_os"], "untouched builtins remain")
}

func TestGet_ReturnsIndependentClonesPerCall(t *testing.T) {
	t.Parallel()

	h := serviceinfo.New(serviceinfo.WithServiceID("node-a"))
	a, err := h.Get(t.Context(), &emptypb.Empty{})
	require.NoError(t, err)
	b, err := h.Get(t.Context(), &emptypb.Empty{})
	require.NoError(t, err)

	require.NotSame(t, a, b, "each Get must return a fresh proto pointer")
	require.NotSame(t, a.SemanticVersion, b.SemanticVersion, "nested messages must also be cloned")

	a.Metadata["mutated"] = "true"
	require.NotContains(t, b.GetMetadata(), "mutated", "mutating one response must not leak into another")
}
