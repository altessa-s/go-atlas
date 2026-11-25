// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package client_test

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/client"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"

	corecontext "github.com/altessa-s/go-atlas-core/context"
)

const echoMethod = "/test.Echo/Get"

// echoBackend is a bufconn gRPC server exposing /test.Echo/Get. The handler
// echoes the request or returns the configured error, and records the
// caller's user agent.
type echoBackend struct {
	lis *bufconn.Listener

	mu        sync.Mutex
	err       error
	userAgent string
}

func startBackend(t *testing.T) *echoBackend {
	t.Helper()

	b := &echoBackend{lis: bufconn.Listen(1 << 20)}
	desc := grpc.ServiceDesc{
		ServiceName: "test.Echo",
		HandlerType: (*any)(nil),
		Methods: []grpc.MethodDesc{{
			MethodName: "Get",
			Handler: func(_ any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
				in := &wrapperspb.StringValue{}
				if err := dec(in); err != nil {
					return nil, err
				}
				md, _ := metadata.FromIncomingContext(ctx)
				b.mu.Lock()
				defer b.mu.Unlock()
				b.userAgent = strings.Join(md.Get("user-agent"), ",")
				if b.err != nil {
					return nil, b.err
				}
				return wrapperspb.String("echo:" + in.GetValue()), nil
			},
		}},
	}
	gs := grpc.NewServer()
	gs.RegisterService(&desc, struct{}{})
	go func() { _ = gs.Serve(b.lis) }()
	t.Cleanup(gs.Stop)
	return b
}

func (b *echoBackend) setErr(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.err = err
}

func (b *echoBackend) lastUserAgent() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.userAgent
}

func (b *echoBackend) dialer() grpc.DialOption {
	return grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return b.lis.DialContext(ctx) })
}

func newClient(t *testing.T, b *echoBackend, opts ...client.Option) *client.Client {
	t.Helper()
	opts = append([]client.Option{client.WithInsecure(), client.WithoutHealthCheck(), client.WithDialOptions(b.dialer())}, opts...)
	c, err := client.New(t.Context(), "passthrough:///bufnet", opts...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close(context.WithoutCancel(t.Context())) })
	return c
}

func invoke(t *testing.T, c *client.Client, in string) (string, error) {
	t.Helper()
	conn, err := c.GetConnection(t.Context())
	require.NoError(t, err)
	defer c.ReturnConnection(conn)
	out := &wrapperspb.StringValue{}
	err = conn.Invoke(t.Context(), echoMethod, wrapperspb.String(in), out)
	return out.GetValue(), err
}

func TestClient_RoundTripAndAccessors(t *testing.T) {
	t.Parallel()

	b := startBackend(t)
	logger := slog.New(slog.DiscardHandler)
	c := newClient(t, b,
		client.WithAppName(" billing-cli "),
		client.WithLogger(logger),
		client.WithMutationTimeout(3*time.Second),
		client.WithQueryTimeout(2*time.Second),
	)

	got, err := invoke(t, c, "hi")
	require.NoError(t, err)
	require.Equal(t, "echo:hi", got)
	require.True(t, strings.HasPrefix(b.lastUserAgent(), "billing-cli"), b.lastUserAgent())

	require.NoError(t, c.HealthCheck(t.Context()))
	require.Equal(t, "passthrough:///bufnet", c.Address())
	require.Same(t, logger, c.Logger())
	require.Equal(t, 3*time.Second, c.MutationTimeout())
	require.Equal(t, 2*time.Second, c.QueryTimeout())

	mctx, mcancel := c.ApplyMutationTimeout(t.Context())
	defer mcancel()
	deadline, ok := mctx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(3*time.Second), deadline, time.Second)

	qctx, qcancel := c.ApplyQueryTimeout(t.Context())
	defer qcancel()
	deadline, ok = qctx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(2*time.Second), deadline, time.Second)

	tctx, tcancel := corecontext.ApplyTimeout(t.Context(), 0)
	defer tcancel()
	_, ok = tctx.Deadline()
	require.False(t, ok, "zero timeout leaves the context without a deadline")
}

func TestClient_HealthCheckAfterClose(t *testing.T) {
	t.Parallel()

	b := startBackend(t)
	c, err := client.New(t.Context(), "passthrough:///bufnet", client.WithInsecure(), client.WithDialOptions(b.dialer()))
	require.NoError(t, err)
	require.NoError(t, c.Close(t.Context()))

	err = c.HealthCheck(t.Context())
	require.ErrorContains(t, err, "SHUTDOWN")
}

func TestClient_StatusErrorsAreParsed(t *testing.T) {
	t.Parallel()

	b := startBackend(t)
	c := newClient(t, b)

	st, err := status.New(codes.NotFound, "user missing").WithDetails(
		&errdetails.ErrorInfo{Reason: "USER_NOT_FOUND", Metadata: map[string]string{"id": "42"}},
		&errdetails.RequestInfo{RequestId: "req-1"},
	)
	require.NoError(t, err)
	b.setErr(st.Err())

	_, err = invoke(t, c, "x")
	require.Error(t, err)
	require.True(t, client.IsClientError(err))
	ce := client.AsClientError(err)
	require.NotNil(t, ce)
	require.True(t, ce.IsNotFound())
	require.True(t, ce.ReasonIs("USER_NOT_FOUND"))
	require.Equal(t, "req-1", ce.RequestID)
	require.Equal(t, map[string]string{"id": "42"}, ce.ReasonData)
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestClient_CustomErrorConverter(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("domain: unavailable")
	b := startBackend(t)
	c := newClient(t, b, client.WithErrorConverter(func(_ context.Context, st *status.Status) error {
		if st.Code() == codes.Unavailable {
			return sentinel
		}
		return st.Err()
	}))

	b.setErr(status.Error(codes.Unavailable, "down"))
	_, err := invoke(t, c, "x")
	require.ErrorIs(t, err, sentinel)
}

func TestError_CodePredicates(t *testing.T) {
	t.Parallel()

	preds := map[codes.Code]func(*client.Error) bool{
		codes.NotFound:           (*client.Error).IsNotFound,
		codes.AlreadyExists:      (*client.Error).IsAlreadyExists,
		codes.PermissionDenied:   (*client.Error).IsPermissionDenied,
		codes.Unauthenticated:    (*client.Error).IsUnauthenticated,
		codes.Internal:           (*client.Error).IsInternal,
		codes.Unavailable:        (*client.Error).IsUnavailable,
		codes.FailedPrecondition: (*client.Error).IsFailedPrecondition,
		codes.DeadlineExceeded:   (*client.Error).IsDeadlineExceeded,
		codes.Canceled:           (*client.Error).IsCanceled,
		codes.ResourceExhausted:  (*client.Error).IsResourceExhausted,
		codes.InvalidArgument:    (*client.Error).IsInvalidArgument,
		codes.Aborted:            (*client.Error).IsAborted,
		codes.OutOfRange:         (*client.Error).IsOutOfRange,
		codes.Unimplemented:      (*client.Error).IsUnimplemented,
		codes.DataLoss:           (*client.Error).IsDataLoss,
	}
	for code := range preds {
		ce := client.ParseStatusError(status.New(code, "m"))
		require.Equal(t, code, ce.Code())
		for other, pred := range preds {
			require.Equal(t, other == code, pred(ce), "%s predicate on %s error", other, code)
		}
	}
}

func TestParseStatusError_Converters(t *testing.T) {
	t.Parallel()

	st, err := status.New(codes.InvalidArgument, "bad").WithDetails(&errdetails.BadRequest{
		FieldViolations: []*errdetails.BadRequest_FieldViolation{{Field: "email", Description: "invalid"}},
	})
	require.NoError(t, err)

	skip := func(any) []client.FieldError { return nil }
	custom := func(d any) []client.FieldError {
		if _, ok := d.(*errdetails.BadRequest); ok {
			return []client.FieldError{{Field: "custom", Message: "converted"}}
		}
		return nil
	}

	ce := client.ParseStatusError(st, skip, custom)
	require.Equal(t, []client.FieldError{{Field: "custom", Message: "converted"}}, ce.Fields)
	require.True(t, ce.IsValidationError())

	ce = client.ParseStatusError(st, skip)
	require.Equal(t, []client.FieldError{{Field: "email", Message: "invalid"}}, ce.Fields, "nil converter results fall back to default parsing")
}

func TestOptions_BlankValuesKeepDefaults(t *testing.T) {
	t.Parallel()

	b := startBackend(t)
	c := newClient(t, b,
		client.WithAppName(""),
		client.WithAppName((*string)(nil)),
		client.WithAppName(new("  ")),
		client.WithHealthServiceName(""),
		client.WithHealthServiceName((*string)(nil)),
		client.WithHealthServiceName(new("svc")),
		client.WithLogger(nil),
		client.WithMutationTimeout(0),
		client.WithQueryTimeout(-time.Second),
	)
	require.NotNil(t, c.Logger())
	require.Positive(t, c.MutationTimeout())
	require.Positive(t, c.QueryTimeout())

	_, err := invoke(t, c, "x")
	require.NoError(t, err)
	require.False(t, strings.HasPrefix(b.lastUserAgent(), " "))
}

type prefixInterceptor struct {
	*interceptors.NoOpClientInterceptor
	prefixes []string
}

func (i prefixInterceptor) Name() string { return i.prefixes[0] }

func (i prefixInterceptor) ClientUnaryInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, next grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		in, ok := req.(*wrapperspb.StringValue)
		if !ok {
			return next(ctx, method, req, reply, cc, opts...)
		}
		return next(ctx, method, wrapperspb.String(i.prefixes[0]+in.GetValue()), reply, cc, opts...)
	}
}

func TestWithInterceptors(t *testing.T) {
	t.Parallel()
	c := newClient(t, startBackend(t),
		client.WithInterceptors(nil, (*prefixInterceptor)(nil), prefixInterceptor{prefixes: []string{"first/"}}),
		client.WithInterceptors(prefixInterceptor{prefixes: []string{"second/"}}),
	)
	got, err := invoke(t, c, "request")
	require.NoError(t, err)
	require.Equal(t, "echo:second/first/request", got)
}
