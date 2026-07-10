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

	serviceinfov1 "github.com/altessa-s/proto-gen-go/io/altessa/serviceinfo/v1"
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
	out := h.Snapshot(t.Context())
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

// Handler embeds UnimplementedServiceInfoServiceServer, so an RPC method whose
// signature drifts from the generated interface still compiles. The assertion
// below is what catches that.
func TestGetServiceInfo_ReturnsServiceInfo(t *testing.T) {
	t.Parallel()

	var _ serviceinfov1.ServiceInfoServiceServer = serviceinfo.New()

	h := serviceinfo.New(serviceinfo.WithServiceID("node-a"))
	resp, err := h.GetServiceInfo(t.Context(), &serviceinfov1.GetServiceInfoRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp.GetServiceInfo())
	require.Equal(t, appinfo.Name, resp.GetServiceInfo().GetServiceName())
}

func TestNew_OptionalFieldsOmittedWhenEmpty(t *testing.T) {
	t.Parallel()

	out := serviceinfo.New().Snapshot(t.Context())

	require.Nil(t, out.ServiceDescription)
	require.Nil(t, out.ServiceId)
	// Note: ProtoVersion field was removed from serviceinfov1.ServiceInfo
	// If proto version info is needed, it should be added through WithExtraMetadata
}

func TestSnapshot_LeaderProviderIsLeader(t *testing.T) {
	t.Parallel()

	leader := &stubLeader{isLeader: true}
	h := serviceinfo.New(
		serviceinfo.WithServiceID("node-a"),
		serviceinfo.WithLeaderProvider(leader),
	)

	out := h.Snapshot(t.Context())
	require.True(t, out.GetLeader())
	require.Equal(t, "node-a", out.GetLeaderId(), "leader uses own serviceID without consulting LeaderId(ctx)")
	require.Equal(t, 0, leader.calls, "LeaderId(ctx) must not be called when this instance is leader")
}

func TestSnapshot_LeaderProviderNotLeader(t *testing.T) {
	t.Parallel()

	leader := &stubLeader{isLeader: false, leaderID: "node-b"}
	h := serviceinfo.New(
		serviceinfo.WithServiceID("node-a"),
		serviceinfo.WithLeaderProvider(leader),
	)

	out := h.Snapshot(t.Context())
	require.False(t, out.GetLeader())
	require.Equal(t, "node-b", out.GetLeaderId())
	require.Equal(t, 1, leader.calls)
}

func TestSnapshot_LeaderProviderErrorFallsBackToServiceID(t *testing.T) {
	t.Parallel()

	leader := &stubLeader{isLeader: false, err: errors.New("kv timeout")}
	h := serviceinfo.New(
		serviceinfo.WithServiceID("node-a"),
		serviceinfo.WithLeaderProvider(leader),
	)

	out := h.Snapshot(t.Context())
	require.Equal(t, "node-a", out.GetLeaderId(), "error from LeaderId(ctx) -> fall back to own serviceID")
}

func TestSnapshot_LeaderProviderEmptyLeaderIDFallsBackToServiceID(t *testing.T) {
	t.Parallel()

	leader := &stubLeader{isLeader: false, leaderID: ""}
	h := serviceinfo.New(
		serviceinfo.WithServiceID("node-a"),
		serviceinfo.WithLeaderProvider(leader),
	)

	out := h.Snapshot(t.Context())
	require.Equal(t, "node-a", out.GetLeaderId())
}

func TestSnapshot_LeaderFunc(t *testing.T) {
	t.Parallel()

	var ctxSeen context.Context
	h := serviceinfo.New(
		serviceinfo.WithServiceID("node-a"),
		serviceinfo.WithLeader(func(ctx context.Context) (bool, string) {
			ctxSeen = ctx
			return false, "node-c"
		}),
	)

	out := h.Snapshot(t.Context())
	require.False(t, out.GetLeader())
	require.Equal(t, "node-c", out.GetLeaderId())
	require.NotNil(t, ctxSeen, "request context must reach the leader func")
}

func TestWithLeader_NilFnIsNoOp(t *testing.T) {
	t.Parallel()

	h := serviceinfo.New(serviceinfo.WithLeader(nil))
	out := h.Snapshot(t.Context())
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

	out := h.Snapshot(t.Context())
	require.False(t, out.GetLeader(), "WithLeader applied last must win over WithLeaderProvider")
	require.Equal(t, "func-leader", out.GetLeaderId())
}

func TestWithMetadata_OverridesBuiltins(t *testing.T) {
	t.Parallel()

	h := serviceinfo.New(serviceinfo.WithExtraMetadata(map[string]string{
		"go_version": "custom-go",
		"region":     "eu-west-1",
	}))

	out := h.Snapshot(t.Context())
	md := out.GetMetadata()
	require.Equal(t, "custom-go", md["go_version"], "extra metadata wins on conflict")
	require.Equal(t, "eu-west-1", md["region"])
	require.Equal(t, runtime.GOOS, md["go_os"], "untouched builtins remain")
}

func TestSnapshot_ReturnsIndependentClonesPerCall(t *testing.T) {
	t.Parallel()

	h := serviceinfo.New(serviceinfo.WithServiceID("node-a"))
	a := h.Snapshot(t.Context())
	b := h.Snapshot(t.Context())

	require.NotSame(t, a, b, "each Snapshot must return a fresh proto pointer")
	require.NotSame(t, a.SemanticVersion, b.SemanticVersion, "nested messages must also be cloned")

	a.Metadata["mutated"] = "true"
	require.NotContains(t, b.GetMetadata(), "mutated", "mutating one response must not leak into another")
}
