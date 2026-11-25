// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcstatus_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/internal/grpcstatus"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type errorStream struct {
	grpc.ClientStream
	err error
}

func (s errorStream) SendMsg(any) error { return s.err }
func (s errorStream) RecvMsg(any) error { return s.err }
func (s errorStream) CloseSend() error  { return s.err }

func TestStreamErrorConversion(t *testing.T) {
	t.Parallel()
	mapped := errors.New("mapped status")
	for _, tc := range []struct {
		name    string
		wireErr error
		wantErr error
	}{
		{name: "custom", wireErr: status.Error(codes.Aborted, "aborted"), wantErr: mapped},
		{name: "builtin", wireErr: status.Error(codes.Canceled, "canceled"), wantErr: context.Canceled},
		{name: "eof", wireErr: io.EOF, wantErr: io.EOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			intercept := grpcstatus.New(nil, false, func(_ context.Context, st *status.Status) (error, bool) {
				return mapped, st.Code() == codes.Aborted
			}).ClientStreamInterceptor()
			stream, err := intercept(t.Context(), &grpc.StreamDesc{}, nil, "/example/Method",
				func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
					return errorStream{err: tc.wireErr}, nil
				})
			require.NoError(t, err)
			require.ErrorIs(t, stream.SendMsg(nil), tc.wantErr)
			require.ErrorIs(t, stream.RecvMsg(nil), tc.wantErr)
			require.ErrorIs(t, stream.CloseSend(), tc.wantErr)

			_, err = intercept(t.Context(), &grpc.StreamDesc{}, nil, "/example/Method",
				func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
					return nil, tc.wireErr
				})
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestClientErrorLogging(t *testing.T) {
	t.Parallel()
	for _, err := range []error{
		errors.New("Bearer client-callback-token https://remote.test/?api_key=SECRET"),
		status.Error(codes.Aborted, "Bearer client-callback-token https://remote.test/?api_key=SECRET"),
	} {
		t.Run(status.Code(err).String(), func(t *testing.T) {
			t.Parallel()
			var logs bytes.Buffer
			log := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			intercept := grpcstatus.New(log, false, nil).ClientUnaryInterceptor()
			invoke := func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
				return err
			}
			got := intercept(t.Context(), "/example/Method", nil, nil, nil, invoke)
			require.ErrorIs(t, got, err)
			require.Contains(t, logs.String(), "handling client error conversion")
			require.NotContains(t, logs.String(), "client-callback-token")
			require.NotContains(t, logs.String(), "SECRET")

			logs.Reset()
			intercept = grpcstatus.New(log, true, nil).ClientUnaryInterceptor()
			got = intercept(t.Context(), "/example/Method", nil, nil, nil, invoke)
			require.ErrorIs(t, got, err)
			require.Contains(t, logs.String(), "client-callback-token")
		})
	}
}
