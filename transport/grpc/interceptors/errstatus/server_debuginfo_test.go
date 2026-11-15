// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

// A status the handler returned directly with a client-facing code keeps its
// message and every detail except DebugInfo.
func TestServerInterceptor_DirectClientStatusDropsDebugInfo(t *testing.T) {
	t.Parallel()

	custom, err := structpb.NewStruct(map[string]any{"tenant": "acme"})
	require.NoError(t, err)
	st := statusWith(t, codes.NotFound, "user not found",
		&errdetails.ErrorInfo{Reason: "USER_NOT_FOUND"}, debugInfo(), custom, debugInfo())

	for name, handlerErr := range map[string]error{
		"status":             st.Err(),
		"interceptors.Error": interceptors.NewError(st, errors.New("cause")),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var logs bytes.Buffer
			si := ServerInterceptor(
				WithLogger(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))),
			).(*interceptor)

			_, err := callUnary(t, si, handlerErr)

			got := status.Convert(err)
			require.Equal(t, codes.NotFound, got.Code())
			require.Equal(t, "user not found", got.Message())
			require.Equal(t, []string{"google.rpc.ErrorInfo", "google.protobuf.Struct"}, detailNames(t, err))
			require.NotContains(t, got.Proto().String(), "hunter2")
			require.Contains(t, logs.String(), "gRPC status details dropped")
			require.Contains(t, logs.String(), "google.rpc.DebugInfo")
			require.Len(t, st.Details(), 4, "the handler's status is not mutated")
		})
	}
}

// A direct client-facing status without DebugInfo is returned as is.
func TestServerInterceptor_DirectClientStatusWithoutDebugInfoIsUntouched(t *testing.T) {
	t.Parallel()
	si := ServerInterceptor().(*interceptor)
	st := statusWith(t, codes.NotFound, "user not found", &errdetails.ErrorInfo{Reason: "USER_NOT_FOUND"})

	got, stripped := withoutDebugInfo(st)
	require.False(t, stripped)
	require.Same(t, st, got)

	_, err := callUnary(t, si, st.Err())
	require.Equal(t, []string{"google.rpc.ErrorInfo"}, detailNames(t, err))
}

// The DebugInfo check must cost nothing for a status without details, the
// common case. Serial: testing.AllocsPerRun must not run beside parallel tests.
func TestWithoutDebugInfo_NoDetailsDoesNotAllocate(t *testing.T) { //nolint:paralleltest // AllocsPerRun forbids parallel tests
	st := status.New(codes.NotFound, "user not found")

	allocs := testing.AllocsPerRun(100, func() {
		_, _ = withoutDebugInfo(st)
	})

	require.Zero(t, allocs)
}
