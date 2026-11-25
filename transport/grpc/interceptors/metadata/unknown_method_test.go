// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package metadata

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestUnknownMethodsShareCacheEntry(t *testing.T) {
	t.Parallel()

	const prefix = "/metadata.cardinality/Unknown"
	const known = "/metadata.cardinality/Registered"
	echo := func(_ any, stream grpc.ServerStream) error {
		var request wrapperspb.StringValue
		if err := stream.RecvMsg(&request); err != nil {
			return err
		}
		path, _ := grpc.Method(stream.Context())
		return stream.SendMsg(wrapperspb.String(MethodFromContext(stream.Context()) + " " + path))
	}
	server := grpc.NewServer(
		grpc.UnknownServiceHandler(echo),
		grpc.StreamInterceptor(func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, next grpc.StreamHandler) error {
			_, meta := EnsureServerStreamInContext(srv, stream.Context(), info)
			return next(srv, &callMetadataStream{ServerStream: stream, meta: meta})
		}),
	)
	server.RegisterService(&grpc.ServiceDesc{
		ServiceName: "metadata.cardinality",
		HandlerType: (*any)(nil),
		Streams:     []grpc.StreamDesc{{StreamName: "Registered", ServerStreams: true, ClientStreams: true, Handler: echo}},
	}, struct{}{})
	listener := bufconn.Listen(1 << 20)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	checkMethod := func(path, expected string) {
		t.Helper()
		var reply wrapperspb.StringValue
		require.NoError(t, conn.Invoke(t.Context(), path, wrapperspb.String("request"), &reply))
		require.Equal(t, expected+" "+path, reply.GetValue())
	}
	checkMethod(known, known)
	for n := range 1000 {
		checkMethod(fmt.Sprintf("%s%d", prefix, n), UnknownMethod)
	}

	var relevantEntries int
	methodCache.Range(func(key, _ any) bool {
		method := key.(string)
		if method == UnknownMethod || strings.HasPrefix(method, prefix) {
			relevantEntries++
		}
		return true
	})
	require.Equal(t, 1, relevantEntries)
}

type callMetadataStream struct {
	grpc.ServerStream
	meta *CallMetadata
}

func (s *callMetadataStream) Context() context.Context {
	return NewContext(s.ServerStream.Context(), s.meta)
}
