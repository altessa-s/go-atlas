// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/proto-gen-go/io/altessa/badrequest/v1"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/protoadapt"
	"google.golang.org/protobuf/types/known/durationpb"
)

const stackSecret = "goroutine 1 [running]: db.go:42 dsn=postgres://app:hunter2@10.0.0.7"

func debugInfo() *errdetails.DebugInfo {
	return &errdetails.DebugInfo{StackEntries: []string{stackSecret}, Detail: stackSecret}
}

func statusWith(t *testing.T, code codes.Code, msg string, details ...protoadapt.MessageV1) *status.Status {
	t.Helper()
	st, err := status.New(code, msg).WithDetails(details...)
	require.NoError(t, err)
	return st
}

// detailNames returns the protobuf names of the details of the status of err.
func detailNames(t *testing.T, err error) []string {
	t.Helper()
	st, ok := status.FromError(err)
	require.True(t, ok)
	names := make([]string, 0, len(st.Proto().GetDetails()))
	for _, d := range st.Proto().GetDetails() {
		names = append(names, string(d.MessageName()))
	}
	return names
}

// A server-fault status keeps only the details that make sense on a server
// fault; DebugInfo and request-describing details never reach the client.
func TestServerInterceptor_ServerFaultDropsDiagnosticDetails(t *testing.T) {
	t.Parallel()

	for _, code := range []codes.Code{codes.Internal, codes.Unknown, codes.DataLoss} {
		direct := statusWith(t, code, "boom",
			debugInfo(),
			&errdetails.ErrorInfo{Reason: "DB_DOWN", Metadata: map[string]string{"dsn": stackSecret}},
			&errdetails.ResourceInfo{ResourceName: stackSecret},
			&errdetails.RetryInfo{RetryDelay: durationpb.New(time.Second)},
			&errdetails.RequestInfo{RequestId: "req-1"},
			&errdetails.LocalizedMessage{Locale: "en", Message: "Try again later"},
			&errdetails.Help{Links: []*errdetails.Help_Link{{Url: "https://example.com/status"}}},
		)
		for name, handlerErr := range map[string]error{
			"direct":             direct.Err(),
			"wrapped":            fmt.Errorf("query: %w", direct.Err()),
			"interceptors.Error": interceptors.NewError(direct, errors.New("cause")),
		} {
			t.Run(code.String()+"/"+name, func(t *testing.T) {
				t.Parallel()
				var logs bytes.Buffer
				si := ServerInterceptor(
					WithLogger(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))),
				).(*interceptor)

				_, err := callUnary(t, si, handlerErr)

				require.Equal(t, []string{
					"google.rpc.RetryInfo",
					"google.rpc.RequestInfo",
					"google.rpc.LocalizedMessage",
					"google.rpc.Help",
				}, detailNames(t, err))
				require.NotContains(t, status.Convert(err).Proto().String(), "hunter2")
				require.Contains(t, logs.String(), "gRPC status details dropped")
				require.Contains(t, logs.String(), "google.rpc.DebugInfo")
			})
		}
	}
}

// A wrapped status with a client-facing code keeps its own message and its
// client-safe details; only the wrapper text and DebugInfo are discarded.
func TestServerInterceptor_WrappedClientStatusKeepsDetails(t *testing.T) {
	t.Parallel()

	const wrapper = "store 10.0.0.7:5432"
	tests := []struct {
		name string
		st   *status.Status
		want []string
	}{
		{
			"invalid argument with field violations",
			statusWith(t, codes.InvalidArgument, "name: must not be empty",
				&errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{{Field: "name", Description: "must not be empty"}}},
				&badrequestv1.BadRequest{FieldViolations: []*badrequestv1.FieldViolation{{FieldPath: new("name")}}},
				debugInfo()),
			[]string{"google.rpc.BadRequest", "io.altessa.badrequest.v1.BadRequest"},
		},
		{
			"resource exhausted with retry guidance",
			statusWith(t, codes.ResourceExhausted, "quota exceeded",
				&errdetails.RetryInfo{RetryDelay: durationpb.New(3 * time.Second)},
				&errdetails.QuotaFailure{Violations: []*errdetails.QuotaFailure_Violation{{Subject: "project:1"}}},
				debugInfo()),
			[]string{"google.rpc.RetryInfo", "google.rpc.QuotaFailure"},
		},
		{
			"failed precondition with the remaining client-safe types",
			statusWith(t, codes.FailedPrecondition, "not ready",
				&errdetails.PreconditionFailure{Violations: []*errdetails.PreconditionFailure_Violation{{Type: "TOS"}}},
				&errdetails.ResourceInfo{ResourceType: "book"},
				&errdetails.ErrorInfo{Reason: "NOT_READY"},
				&errdetails.RequestInfo{RequestId: "req-1"},
				&errdetails.LocalizedMessage{Locale: "en", Message: "Not ready"},
				&errdetails.Help{},
				debugInfo()),
			[]string{
				"google.rpc.PreconditionFailure", "google.rpc.ResourceInfo", "google.rpc.ErrorInfo",
				"google.rpc.RequestInfo", "google.rpc.LocalizedMessage", "google.rpc.Help",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			si := ServerInterceptor().(*interceptor)
			handlerErr := fmt.Errorf("%s: %w", wrapper, tc.st.Err())

			_, err := callUnary(t, si, handlerErr)

			st := status.Convert(err)
			require.Equal(t, tc.st.Code(), st.Code())
			require.Equal(t, tc.st.Message(), st.Message())
			require.Equal(t, tc.want, detailNames(t, err))
			require.NotContains(t, st.Proto().String(), "hunter2")
			require.NotContains(t, st.Proto().String(), wrapper)
			require.ErrorIs(t, err, handlerErr)
		})
	}
}

// The details the server keeps reach a client-side status converter intact.
func TestServerInterceptor_KeptDetailsRoundTripThroughClient(t *testing.T) {
	t.Parallel()
	si := ServerInterceptor().(*interceptor)
	inner := statusWith(t, codes.ResourceExhausted, "quota exceeded",
		&errdetails.RetryInfo{RetryDelay: durationpb.New(3 * time.Second)}, debugInfo())
	_, serverErr := callUnary(t, si, fmt.Errorf("limiter: %w", inner.Err()))

	var seen *status.Status
	ci := ClientInterceptor(WithStatusConverterFunc(func(_ context.Context, st *status.Status) error {
		seen = st
		return st.Err()
	}))
	// The wire carries only the status, not the interceptors.Error around it.
	err := convertClientError(t, ci, status.Convert(serverErr).Err())

	require.Equal(t, codes.ResourceExhausted, status.Code(err))
	require.Len(t, seen.Details(), 1)
	retry, ok := seen.Details()[0].(*errdetails.RetryInfo)
	require.True(t, ok)
	require.Equal(t, 3*time.Second, retry.GetRetryDelay().AsDuration())
}
