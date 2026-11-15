// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package audit_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	auditgrpc "github.com/altessa-s/go-atlas/transport/grpc/interceptors/audit"
)

type captureEmitter struct {
	event *audit.Event
}

func (c *captureEmitter) Emit(event *audit.Event) bool {
	c.event = event
	return true
}

func TestServerInterceptor_CapsClientControlledValues(t *testing.T) {
	t.Parallel()

	huge := strings.Repeat("a", 1<<20)
	userAgent := func(context.Context) audit.Actor { return audit.Actor{ID: "u1", UserAgent: huge} }

	tests := []struct {
		name                         string
		opts                         []auditgrpc.Option
		wantMethod, wantUA, wantMsgs int
	}{
		{
			name:       "defaults",
			wantMethod: auditgrpc.DefaultMaxMethodLength,
			wantUA:     auditgrpc.DefaultMaxUserAgentLength,
			wantMsgs:   auditgrpc.DefaultMaxMessageLength,
		},
		{
			name: "configured",
			opts: []auditgrpc.Option{
				auditgrpc.WithMaxMethodLength(64),
				auditgrpc.WithMaxUserAgentLength(32),
				auditgrpc.WithMaxMessageLength(16),
			},
			wantMethod: 64,
			wantUA:     32,
			wantMsgs:   16,
		},
		{
			name: "values below 1 keep the defaults",
			opts: []auditgrpc.Option{
				auditgrpc.WithMaxMethodLength(0),
				auditgrpc.WithMaxUserAgentLength(-1),
				auditgrpc.WithMaxMessageLength(0),
			},
			wantMethod: auditgrpc.DefaultMaxMethodLength,
			wantUA:     auditgrpc.DefaultMaxUserAgentLength,
			wantMsgs:   auditgrpc.DefaultMaxMessageLength,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			emitter := &captureEmitter{}
			opts := append([]auditgrpc.Option{auditgrpc.WithActorExtractor(userAgent)}, tc.opts...)
			interceptor := auditgrpc.ServerInterceptor(emitter, opts...).ServerUnaryInterceptor()

			info := &grpc.UnaryServerInfo{FullMethod: "/" + huge + "/Get"}
			_, err := interceptor(t.Context(), nil, info, func(context.Context, any) (any, error) {
				return nil, status.Error(codes.InvalidArgument, "bad name "+huge)
			})
			require.Equal(t, codes.InvalidArgument, status.Code(err))
			require.Contains(t, status.Convert(err).Message(), huge, "the caller still gets the full status")

			event := emitter.event
			require.NotNil(t, event)
			require.Len(t, event.Resource.Path, tc.wantMethod)
			require.LessOrEqual(t, len(event.Resource.Type), tc.wantMethod)
			require.Len(t, event.Actor.UserAgent, tc.wantUA)
			require.Len(t, event.Result.Message, tc.wantMsgs)
			for _, v := range []string{event.Resource.Path, event.Actor.UserAgent, event.Result.Message} {
				require.True(t, strings.HasSuffix(v, auditgrpc.TruncationSuffix))
			}
		})
	}
}

func TestServerInterceptor_KeepsShortValues(t *testing.T) {
	t.Parallel()

	emitter := &captureEmitter{}
	interceptor := auditgrpc.ServerInterceptor(emitter,
		auditgrpc.WithActorExtractor(func(context.Context) audit.Actor { return audit.Actor{UserAgent: "grpc-go/1.70"} }),
	).ServerUnaryInterceptor()

	info := &grpc.UnaryServerInfo{FullMethod: "/example.v1.Service/Get"}
	_, err := interceptor(t.Context(), nil, info, func(context.Context, any) (any, error) {
		return nil, status.Error(codes.NotFound, "no such item")
	})
	require.Equal(t, codes.NotFound, status.Code(err))

	require.Equal(t, "/example.v1.Service/Get", emitter.event.Resource.Path)
	require.Equal(t, "example.v1.Service", emitter.event.Resource.Type)
	require.Equal(t, "grpc-go/1.70", emitter.event.Actor.UserAgent)
	require.Equal(t, "no such item", emitter.event.Result.Message)
}
