// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// contextStream is a server stream that only carries a context.
type contextStream struct {
	grpc.ServerStream
	ctx context.Context //nolint:containedctx // a ServerStream exposes its context
}

func (s contextStream) Context() context.Context { return s.ctx }

// A service-level converter that returns nil or an OK status suppresses the
// handler's error: the call succeeds with the handler's response, nothing is
// logged as replaced, and the finalizer has no error to decorate.
func TestServerInterceptor_SuppressedErrorIsSuccess(t *testing.T) {
	t.Parallel()

	suppressions := map[string]func(context.Context, error) error{
		"nil":               func(context.Context, error) error { return nil },
		"ok status":         func(context.Context, error) error { return status.New(codes.OK, "fine").Err() },
		"custom ok carrier": func(context.Context, error) error { return okStatusError{} },
	}
	for name, convert := range suppressions {
		svc := &mockStatusErrorService{convertFn: convert}

		t.Run(name+"/unary", func(t *testing.T) {
			t.Parallel()
			var logs bytes.Buffer
			si := ServerInterceptor(
				WithLogger(slog.New(slog.NewTextHandler(&logs, nil))),
				WithFinalizer(DefaultFinalizer),
			).(*interceptor)

			resp, err := si.ServerUnaryInterceptor()(t.Context(), "req", &grpc.UnaryServerInfo{Server: svc},
				func(context.Context, any) (any, error) { return "partial response", errors.New("ignorable") })

			require.NoError(t, err)
			require.Equal(t, "partial response", resp)
			require.Empty(t, logs.String())
		})

		t.Run(name+"/stream", func(t *testing.T) {
			t.Parallel()
			var logs bytes.Buffer
			si := ServerInterceptor(
				WithLogger(slog.New(slog.NewTextHandler(&logs, nil))),
				WithFinalizer(DefaultFinalizer),
			).(*interceptor)

			err := si.ServerStreamInterceptor()(svc, contextStream{ctx: t.Context()}, &grpc.StreamServerInfo{},
				func(any, grpc.ServerStream) error { return errors.New("ignorable") })

			require.NoError(t, err)
			require.Empty(t, logs.String())
		})
	}
}

// okStatusError carries an OK status without being a status error itself.
type okStatusError struct{}

func (okStatusError) Error() string              { return "ok" }
func (okStatusError) GRPCStatus() *status.Status { return status.New(codes.OK, "") }
