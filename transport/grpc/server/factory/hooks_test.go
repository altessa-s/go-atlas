// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldbehavior"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/idempotency"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/logger"
	"github.com/altessa-s/go-atlas/transport/internal/depgraph"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
	idempotencydata "github.com/altessa-s/go-atlas/data/idempotency"
	sharedlimiter "github.com/altessa-s/go-atlas/data/limiters"
	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
	grpcserver "github.com/altessa-s/go-atlas/transport/grpc/server"
	stdGrpc "google.golang.org/grpc"
)

// enabled is the base of an enabled interceptor section.
var enabled = grpcconfig.BaseInterceptor{EnableMixin: middlewareconfig.EnableMixin{Enabled: true}}

// echoMethod is the only method of the test service: it returns its request.
const echoMethod = "/hooks.v1.Echo/Echo"

// echoHandler serves echoMethod, recording the request the chain passes on.
type echoHandler struct {
	mu  sync.Mutex
	got *wrapperspb.StringValue
}

func (h *echoHandler) handle(srv any, ctx context.Context, dec func(any) error, ic stdGrpc.UnaryServerInterceptor) (any, error) {
	in := new(wrapperspb.StringValue)
	if err := dec(in); err != nil {
		return nil, err
	}
	call := func(_ context.Context, req any) (any, error) {
		v, _ := req.(*wrapperspb.StringValue)
		h.mu.Lock()
		h.got = v
		h.mu.Unlock()
		return v, nil
	}
	if ic == nil {
		return call(ctx, in)
	}
	return ic(ctx, in, &stdGrpc.UnaryServerInfo{Server: srv, FullMethod: echoMethod}, call)
}

// serveChain serves the echo service through the builder's interceptor chain
// over an in-memory listener and returns a client connection to it.
func serveChain(t *testing.T, b *ServerBuilder, h *echoHandler) *stdGrpc.ClientConn {
	t.Helper()
	return serveService(t, b, &stdGrpc.ServiceDesc{
		ServiceName: "hooks.v1.Echo",
		HandlerType: (*any)(nil),
		Methods:     []stdGrpc.MethodDesc{{MethodName: "Echo", Handler: h.handle}},
	})
}

// serveService serves desc through the builder's interceptor chain over an
// in-memory listener and returns a client connection to it.
func serveService(t *testing.T, b *ServerBuilder, desc *stdGrpc.ServiceDesc) *stdGrpc.ClientConn {
	t.Helper()
	require.Empty(t, b.errs)

	list := make([]any, 0, len(b.interceptors))
	for _, i := range b.interceptors {
		list = append(list, i)
	}
	opts, err := interceptors.NewChain(list...).ServerOptions()
	require.NoError(t, err)

	ln := bufconn.Listen(1 << 20)
	srv := stdGrpc.NewServer(opts...)
	srv.RegisterService(desc, nil)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(srv.Stop)

	conn, err := stdGrpc.NewClient("passthrough:///bufnet",
		stdGrpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return ln.DialContext(ctx) }),
		stdGrpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestUseLoggerOptions(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		Logger: &grpcconfig.LoggerInterceptor{BaseInterceptor: enabled, LogRequest: true},
	}}
	b := New(cfg).
		UseLogger(slog.New(slog.NewJSONHandler(&buf, nil))).
		UseLoggerOptions(
			logger.WithLogGrpcResponseCodes(uint32(codes.OK)),
			logger.WithPayloadRedactor(func(proto.Message) any { return "[REDACTED]" }),
		).
		WithLoggerInterceptor()

	conn := serveChain(t, b, &echoHandler{})
	var out wrapperspb.StringValue
	require.NoError(t, conn.Invoke(t.Context(), echoMethod, wrapperspb.String("top-secret"), &out))

	logged := buf.String()
	require.Contains(t, logged, "[REDACTED]", "the code-supplied redactor reaches the configured interceptor")
	require.NotContains(t, logged, "top-secret")
}

func TestUseFieldBehaviorOptions(t *testing.T) {
	t.Parallel()

	const method = "/x.v1.X/RecordResource"
	b := New(&grpcconfig.Config{}).
		UseFieldBehaviorOptions(fieldbehavior.WithMethodKind(method, fieldbehavior.KindCreate)).
		WithFieldBehaviorInterceptor()
	require.Len(t, b.interceptors, 1)

	unary := b.interceptors[0].ServerUnaryInterceptor()
	var seen *pb.Resource
	_, err := unary(t.Context(), &pb.Resource{Name: "n", CreateTime: "2026-01-01T00:00:00Z"},
		&stdGrpc.UnaryServerInfo{FullMethod: method},
		func(_ context.Context, req any) (any, error) {
			seen, _ = req.(*pb.Resource)
			return &pb.Resource{}, nil
		})
	require.NoError(t, err)
	require.NotNil(t, seen)
	require.Empty(t, seen.GetCreateTime(), "the method kind classifies the non-standard name as a create")
	require.Equal(t, "n", seen.GetName())
}

func TestUseIdempotencyOptions(t *testing.T) {
	t.Parallel()

	cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		Idempotency: &grpcconfig.IdempotencyInterceptor{
			BaseInterceptor:                enabled,
			IdempotencyKeyHeader:           "idempotency-key",
			IdempotencyKeyStatusMetadata:   "idempotency-key-status",
			IdempotencyKeyEntityIdMetadata: "idempotency-key-entity-id",
			FallbackBehavior:               middlewareconfig.FallbackBehaviorDeny,
			EnforceMandatory:               true,
		},
	}}
	var scenarios []idempotency.ErrorScenario
	b := New(cfg).
		UseIdempotency(idempotencydata.New(testhelpers.NewMockIdempotencyStorage())).
		UseIdempotencyOptions(idempotency.WithStatusCreator(
			func(_ context.Context, s idempotency.ErrorScenario, _ *idempotencydata.State) *status.Status {
				scenarios = append(scenarios, s)
				return status.New(codes.FailedPrecondition, "custom")
			})).
		WithIdempotencyInterceptor()

	conn := serveChain(t, b, &echoHandler{})
	var out wrapperspb.StringValue
	err := conn.Invoke(t.Context(), echoMethod, wrapperspb.String("x"), &out)
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.Equal(t, "custom", status.Convert(err).Message())
	require.Equal(t, []idempotency.ErrorScenario{idempotency.ErrorIDKMissing}, scenarios)
}

func TestBuildRejectsLimiterWithoutRealIP(t *testing.T) {
	t.Parallel()

	limiter := sharedlimiter.Func(func(context.Context) (*sharedlimiter.LimitInfo, error) {
		return &sharedlimiter.LimitInfo{}, nil
	})
	cfg := &grpcconfig.Config{
		ListenAddress: "127.0.0.1:0",
		Interceptors: &grpcconfig.Interceptors{
			Limiter: &grpcconfig.LimiterInterceptor{BaseInterceptor: enabled},
		},
	}

	_, err := New(cfg).UseLimiter(limiter).WithInterceptors().Build()
	require.ErrorIs(t, err, depgraph.ErrMissingDependency, "a missing required interceptor fails Build, not Start")

	cfg.Interceptors.RealIp = &grpcconfig.RealIPInterceptor{BaseInterceptor: enabled}
	srv, err := New(cfg).UseLimiter(limiter).WithInterceptors().Build()
	require.NoError(t, err)
	require.NotNil(t, srv)
}

// interceptorNames returns the names of the builder's interceptors.
func interceptorNames(b *ServerBuilder) []string {
	names := make([]string, 0, len(b.interceptors))
	for _, i := range b.interceptors {
		names = append(names, i.Name())
	}
	return names
}

func TestWithFieldMaskInterceptor_ConfigGated(t *testing.T) {
	t.Parallel()

	off := New(&grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{}}).WithInterceptors()
	require.NotContains(t, interceptorNames(off), fieldmask.Name(), "absent section: not wired")

	cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		FieldMask: &grpcconfig.FieldMaskInterceptor{BaseInterceptor: enabled},
	}}
	on := New(cfg).WithInterceptors()
	require.Contains(t, interceptorNames(on), fieldmask.Name(), "WithInterceptors wires the enabled section")

	excluded := New(cfg).WithInterceptors(fieldmask.ID)
	require.NotContains(t, interceptorNames(excluded), fieldmask.Name())
}

func TestWithFieldMaskInterceptor_ConfigAndCodeOptions(t *testing.T) {
	t.Parallel()

	const modify = "/x.v1.X/ModifyResource"
	cfg := &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		FieldMask: &grpcconfig.FieldMaskInterceptor{BaseInterceptor: enabled, ApplyEmptyUpdateMask: true},
	}}
	b := New(cfg).
		UseFieldMaskOptions(fieldmask.WithMethodKind(modify, fieldmask.KindUpdate)).
		WithFieldMaskInterceptor()
	require.Empty(t, b.errs)
	require.Len(t, b.interceptors, 1)
	unary := b.interceptors[0].ServerUnaryInterceptor()

	call := func(method string, req *pb.UpdateResourceRequest) (*pb.UpdateResourceRequest, error) {
		var seen *pb.UpdateResourceRequest
		_, err := unary(t.Context(), req, &stdGrpc.UnaryServerInfo{FullMethod: method},
			func(_ context.Context, r any) (any, error) {
				seen, _ = r.(*pb.UpdateResourceRequest)
				return &pb.Resource{}, nil
			})
		return seen, err
	}

	// Code-supplied method kind: the unconventional name is treated as an update, so
	// a mask over an IMMUTABLE field is rejected.
	_, err := call(modify, &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Name: "n", TenantId: "altered"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"tenant_id"}},
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	// applyEmptyUpdateMask from config: an empty mask keeps the populated
	// fields instead of clearing the resource.
	seen, err := call("/x.v1.X/UpdateResource", &pb.UpdateResourceRequest{
		Resource:   &pb.Resource{Name: "n", Description: "d"},
		UpdateMask: &fieldmaskpb.FieldMask{},
	})
	require.NoError(t, err)
	require.Equal(t, "n", seen.GetResource().GetName())
	require.Equal(t, "d", seen.GetResource().GetDescription())
}

func TestUseServerOptions(t *testing.T) {
	t.Parallel()

	ln := bufconn.Listen(1 << 20)
	srv, err := New(&grpcconfig.Config{ListenAddress: "127.0.0.1:0"}).
		UseServerOptions(grpcserver.WithListener(ln), grpcserver.WithStartupVerification(0)).
		Build()
	require.NoError(t, err)

	start := time.Now()
	require.NoError(t, srv.Start())
	require.Less(t, time.Since(start), time.Second, "no startup wait")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
		defer cancel()
		require.NoError(t, srv.Shutdown(ctx))
	})
	require.Equal(t, ln.Addr().String(), srv.Address(), "the injected listener is served")
}

// TestIdempotencyKeySourceFromConfig checks that the key field, key format
// and method classification of the config reach the interceptor.
func TestIdempotencyKeySourceFromConfig(t *testing.T) {
	t.Parallel()

	section := func(mutate func(*grpcconfig.IdempotencyInterceptor)) *grpcconfig.Config {
		c := grpcconfig.DefaultIdempotencyInterceptor()
		c.BaseInterceptor = enabled
		mutate(&c)
		return &grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{Idempotency: &c}}
	}
	invoke := func(t *testing.T, conn *stdGrpc.ClientConn, value string) error {
		t.Helper()
		var out wrapperspb.StringValue
		return conn.Invoke(t.Context(), echoMethod, wrapperspb.String(value), &out)
	}

	t.Run("field, opaque format, mandatory method", func(t *testing.T) {
		t.Parallel()
		b := New(section(func(c *grpcconfig.IdempotencyInterceptor) {
			c.IdempotencyKeyField = "value"
			c.KeyFormat = middlewareconfig.IdempotencyKeyFormatOpaque
			c.MandatoryMethods = []string{echoMethod}
		})).UseIdempotency(idempotencydata.New(testhelpers.NewMockIdempotencyStorage())).WithIdempotencyInterceptor()
		conn := serveChain(t, b, &echoHandler{})

		require.Equal(t, codes.InvalidArgument, status.Code(invoke(t, conn, "")), "the listed method requires a key")
		require.NoError(t, invoke(t, conn, "client-req-1"), "an opaque key from the request field")
		require.Equal(t, codes.FailedPrecondition, status.Code(invoke(t, conn, "client-req-1")))
		require.Equal(t, codes.InvalidArgument, status.Code(invoke(t, conn, "has space")))
	})

	t.Run("verbs classifier", func(t *testing.T) {
		t.Parallel()
		b := New(section(func(c *grpcconfig.IdempotencyInterceptor) {
			c.MethodClassifier = grpcconfig.IdempotencyMethodClassifierVerbs
			c.EnforceMandatory = true
		})).UseIdempotency(idempotencydata.New(testhelpers.NewMockIdempotencyStorage())).WithIdempotencyInterceptor()
		conn := serveChain(t, b, &echoHandler{})
		require.Equal(t, codes.InvalidArgument, status.Code(invoke(t, conn, "x")), "Echo is a custom verb")
	})

	t.Run("invalid pattern", func(t *testing.T) {
		t.Parallel()
		b := New(section(func(c *grpcconfig.IdempotencyInterceptor) {
			c.KeyFormat = middlewareconfig.IdempotencyKeyFormatPattern
			c.KeyPattern = "("
		})).UseIdempotency(idempotencydata.New(testhelpers.NewMockIdempotencyStorage())).WithIdempotencyInterceptor()
		require.NotEmpty(t, b.errs)
		require.NotContains(t, b.errs[0].Error(), "(", "the pattern is not quoted")
	})
}

// TestIdempotencyResponseReplayFromConfig checks that responseReplay reaches
// the interceptor: the completed key stores a replay record bound to the
// method. (The echo method is not a registered protobuf method, so its
// repeat is still refused: a response is replayed only when its type is the
// method's registered response type.)
func TestIdempotencyResponseReplayFromConfig(t *testing.T) {
	t.Parallel()

	c := grpcconfig.DefaultIdempotencyInterceptor()
	c.BaseInterceptor = enabled
	c.IdempotencyKeyField = "value"
	c.KeyFormat = middlewareconfig.IdempotencyKeyFormatOpaque
	c.ResponseReplay = true
	keeper := idempotencydata.New(testhelpers.NewMockIdempotencyStorage())
	b := New(&grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{Idempotency: &c}}).
		UseIdempotency(keeper).WithIdempotencyInterceptor()
	conn := serveChain(t, b, &echoHandler{})

	var out wrapperspb.StringValue
	require.NoError(t, conn.Invoke(t.Context(), echoMethod, wrapperspb.String("req-1"), &out))
	locked, state, err := keeper.AttemptLock(t.Context(), idempotencydata.ComposeKey("transport", "hooks.v1.Echo", "req-1"))
	require.NoError(t, err)
	require.False(t, locked)
	data, ok := state.Data.(map[string]any)
	require.True(t, ok, "a replay record, not a bare entity ID: %T", state.Data)
	require.Equal(t, echoMethod, data["binding"])
	require.NotEmpty(t, data["response"])
}
