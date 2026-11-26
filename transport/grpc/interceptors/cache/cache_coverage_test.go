// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/cache"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/wrapperspb"

	grpccache "github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"
	compression "github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache/compression"
)

const echoMethod = "/test.Echo/Get"

// mapCacher is a JSON-backed in-memory [cache.Cacher] that records keys
// and can be told to fail.
type mapCacher struct {
	mu      sync.Mutex
	entries map[string][]byte
	getErr  error
	saveErr error
	gets    int
	saves   int
}

func newMapCacher() *mapCacher { return &mapCacher{entries: map[string][]byte{}} }

func (c *mapCacher) Save(_ context.Context, key string, v any, _ ...time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.saves++
	if c.saveErr != nil {
		return c.saveErr
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.entries[key] = b
	return nil
}

func (c *mapCacher) Get(_ context.Context, key string, v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gets++
	if c.getErr != nil {
		return c.getErr
	}
	b, ok := c.entries[key]
	if !ok {
		return cache.ErrMissing
	}
	return json.Unmarshal(b, v)
}

func (c *mapCacher) Exists(_ context.Context, key string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.entries[key]
	return ok, nil
}

func (c *mapCacher) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
	return nil
}

func (c *mapCacher) DeleteMany(ctx context.Context, keys ...string) error {
	for _, k := range keys {
		_ = c.Delete(ctx, k)
	}
	return nil
}

func (c *mapCacher) keys() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.entries))
	for k := range c.entries {
		out = append(out, k)
	}
	return out
}

func (c *mapCacher) corruptAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.entries {
		c.entries[k] = []byte(`{"data":"bm90LWEtcHJvdG8tYnV0LXN0aWxsLWJ5dGVz/w=="}`)
	}
}

// echoServer serves /test.Echo/Get through the given interceptor and counts
// handler invocations.
type echoServer struct {
	conn  *grpc.ClientConn
	calls atomic.Int32
}

func startEcho(t *testing.T, unary grpc.UnaryServerInterceptor, handle func(ctx context.Context, in *wrapperspb.StringValue) (any, error)) *echoServer {
	t.Helper()

	es := &echoServer{}
	desc := grpc.ServiceDesc{
		ServiceName: "test.Echo",
		HandlerType: (*any)(nil),
		Methods: []grpc.MethodDesc{{
			MethodName: "Get",
			Handler: func(srv any, ctx context.Context, dec func(any) error, ic grpc.UnaryServerInterceptor) (any, error) {
				in := &wrapperspb.StringValue{}
				if err := dec(in); err != nil {
					return nil, err
				}
				h := func(ctx context.Context, req any) (any, error) {
					es.calls.Add(1)
					return handle(ctx, req.(*wrapperspb.StringValue))
				}
				if ic == nil {
					return h(ctx, in)
				}
				return ic(ctx, in, &grpc.UnaryServerInfo{Server: srv, FullMethod: echoMethod}, h)
			},
		}},
	}

	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer(grpc.ChainUnaryInterceptor(unary))
	gs.RegisterService(&desc, struct{}{})
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	es.conn = conn
	return es
}

func (es *echoServer) call(ctx context.Context, in string) (string, metadata.MD, error) {
	var hdr metadata.MD
	out := &wrapperspb.StringValue{}
	err := es.conn.Invoke(ctx, echoMethod, wrapperspb.String(in), out, grpc.Header(&hdr))
	return out.GetValue(), hdr, err
}

func echo(_ context.Context, in *wrapperspb.StringValue) (any, error) {
	return wrapperspb.String("echo:" + in.GetValue()), nil
}

func TestName(t *testing.T) {
	t.Parallel()
	require.Equal(t, "cache", grpccache.Name())
	require.Equal(t, "cache", grpccache.ID.Name())

	si := grpccache.ServerInterceptor(newMapCacher())
	require.Equal(t, "cache", si.Name())
	deps, ok := si.(interface{ Dependencies() []string })
	require.True(t, ok)
	require.Equal(t, []string{"metadata", "auth", "ipacl", "geoacl", "limiter"}, deps.Dependencies())
}

func TestCache_MissThenHitWithHeaders(t *testing.T) {
	t.Parallel()

	cacher := newMapCacher()
	si := grpccache.ServerInterceptor(cacher,
		grpccache.WithMethod(echoMethod, &wrapperspb.StringValue{}),
		grpccache.WithPublicMethods(echoMethod),
		grpccache.WithCacheHeadersEnabled(),
		grpccache.WithKeysPrefix("svc:"),
		grpccache.WithCacheTTL(time.Minute),
		grpccache.WithLogger(nil),
	)
	es := startEcho(t, si.ServerUnaryInterceptor(), echo)

	got, hdr, err := es.call(t.Context(), "a")
	require.NoError(t, err)
	require.Equal(t, "echo:a", got)
	require.Equal(t, []string{grpccache.HeaderMiss}, hdr.Get(grpccache.HeaderKey))

	got, hdr, err = es.call(t.Context(), "a")
	require.NoError(t, err)
	require.Equal(t, "echo:a", got)
	require.Equal(t, []string{grpccache.HeaderHit}, hdr.Get(grpccache.HeaderKey))
	require.Equal(t, int32(1), es.calls.Load(), "second call is served from cache")

	_, _, err = es.call(t.Context(), "b")
	require.NoError(t, err)
	require.Equal(t, int32(2), es.calls.Load(), "different request is a different key")

	keys := cacher.keys()
	require.Len(t, keys, 2)
	for _, k := range keys {
		require.True(t, strings.HasPrefix(k, "svc:"), k)
	}
}

func TestCache_HeadersDisabled(t *testing.T) {
	t.Parallel()

	si := grpccache.ServerInterceptor(newMapCacher(),
		grpccache.WithMethod(echoMethod, &wrapperspb.StringValue{}),
		grpccache.WithPublicMethods(echoMethod),
		grpccache.WithCacheHeaders(false),
	)
	es := startEcho(t, si.ServerUnaryInterceptor(), echo)

	_, hdr, err := es.call(t.Context(), "a")
	require.NoError(t, err)
	require.Empty(t, hdr.Get(grpccache.HeaderKey))
}

func TestCache_ErrorResponsesCachedWhenDecisionAllows(t *testing.T) {
	t.Parallel()

	si := grpccache.ServerInterceptor(newMapCacher(),
		grpccache.WithMethod(echoMethod, &wrapperspb.StringValue{}),
		grpccache.WithPublicMethods(echoMethod),
		grpccache.WithCacheDecision(grpccache.DefaultSuccessAndErrorDecision(time.Minute, time.Minute)),
	)
	es := startEcho(t, si.ServerUnaryInterceptor(), func(context.Context, *wrapperspb.StringValue) (any, error) {
		return nil, status.Error(codes.NotFound, "no such thing")
	})

	for range 2 {
		_, _, err := es.call(t.Context(), "x")
		st := status.Convert(err)
		require.Equal(t, codes.NotFound, st.Code())
		require.Equal(t, "no such thing", st.Message())
	}
	require.Equal(t, int32(1), es.calls.Load(), "cached NotFound served without calling the handler")
}

func TestCache_ErrorsNotCachedByDefault(t *testing.T) {
	t.Parallel()

	si := grpccache.ServerInterceptor(newMapCacher(),
		grpccache.WithMethod(echoMethod, &wrapperspb.StringValue{}), grpccache.WithPublicMethods(echoMethod))
	es := startEcho(t, si.ServerUnaryInterceptor(), func(context.Context, *wrapperspb.StringValue) (any, error) {
		return nil, status.Error(codes.Unavailable, "down")
	})

	for range 2 {
		_, _, err := es.call(t.Context(), "x")
		require.Equal(t, codes.Unavailable, status.Code(err))
	}
	require.Equal(t, int32(2), es.calls.Load())
}

func TestCache_BypassCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts []grpccache.Option
	}{
		{name: "method not configured"},
		{name: "ignored method", opts: []grpccache.Option{
			grpccache.WithMethod(echoMethod, &wrapperspb.StringValue{}), grpccache.WithIgnoreMethods(" " + echoMethod + " "),
		}},
		{name: "ignored pattern", opts: []grpccache.Option{
			grpccache.WithMethod(echoMethod, &wrapperspb.StringValue{}), grpccache.WithIgnorePatterns(regexp.MustCompile(`^/test\.echo/`)),
		}},
		{name: "empty method config skipped", opts: []grpccache.Option{
			grpccache.WithMethod("", &wrapperspb.StringValue{}), grpccache.WithMethod(echoMethod, nil),
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cacher := newMapCacher()
			opts := append([]grpccache.Option{grpccache.WithPublicMethods(echoMethod)}, tc.opts...)
			es := startEcho(t, grpccache.ServerInterceptor(cacher, opts...).ServerUnaryInterceptor(), echo)
			for range 2 {
				got, _, err := es.call(t.Context(), "a")
				require.NoError(t, err)
				require.Equal(t, "echo:a", got)
			}
			require.Equal(t, int32(2), es.calls.Load())
			require.Empty(t, cacher.keys())
		})
	}
}

func TestCache_WithMethodsMap(t *testing.T) {
	t.Parallel()

	si := grpccache.ServerInterceptor(newMapCacher(), grpccache.WithPublicMethods(echoMethod), grpccache.WithMethods(map[string]*grpccache.MethodPolicy{
		strings.ToLower(echoMethod): {Name: echoMethod, ResponseProto: &wrapperspb.StringValue{}},
	}))
	es := startEcho(t, si.ServerUnaryInterceptor(), echo)
	for range 2 {
		_, _, err := es.call(t.Context(), "a")
		require.NoError(t, err)
	}
	require.Equal(t, int32(1), es.calls.Load())
}

func TestCache_CompressionRoundTrip(t *testing.T) {
	t.Parallel()

	big := strings.Repeat("compressible-payload-", 512)
	tests := []struct {
		name string
		opts []grpccache.Option
	}{
		{name: "custom compression", opts: []grpccache.Option{
			grpccache.WithMethod(echoMethod, &wrapperspb.StringValue{}), grpccache.WithCompression(16, 0, 5),
		}},
		{name: "preset best", opts: []grpccache.Option{
			grpccache.WithMethod(echoMethod, &wrapperspb.StringValue{}), grpccache.WithCompressionPreset(compression.PresetBest),
		}},
		{name: "preset fast", opts: []grpccache.Option{
			grpccache.WithMethod(echoMethod, &wrapperspb.StringValue{}), grpccache.WithCompressionPreset(compression.PresetFast),
		}},
		{name: "preset none", opts: []grpccache.Option{
			grpccache.WithMethod(echoMethod, &wrapperspb.StringValue{}), grpccache.WithCompressionPreset(compression.PresetNone),
		}},
		{name: "per-method balanced", opts: []grpccache.Option{grpccache.WithMethodPolicy(grpccache.MethodPolicy{
			Name: echoMethod, ResponseProto: &wrapperspb.StringValue{}, CompressionPreset: compression.PresetBalanced,
		})}},
		{name: "per-method best", opts: []grpccache.Option{grpccache.WithMethodPolicy(grpccache.MethodPolicy{
			Name: echoMethod, ResponseProto: &wrapperspb.StringValue{}, CompressionPreset: compression.PresetBest,
		})}},
		{name: "per-method fast", opts: []grpccache.Option{grpccache.WithMethodPolicy(grpccache.MethodPolicy{
			Name: echoMethod, ResponseProto: &wrapperspb.StringValue{}, CompressionPreset: compression.PresetFast,
		})}},
		{name: "per-method none", opts: []grpccache.Option{grpccache.WithMethodPolicy(grpccache.MethodPolicy{
			Name: echoMethod, ResponseProto: &wrapperspb.StringValue{}, CompressionPreset: compression.PresetNone,
		})}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := append([]grpccache.Option{grpccache.WithPublicMethods(echoMethod)}, tc.opts...)
			es := startEcho(t, grpccache.ServerInterceptor(newMapCacher(), opts...).ServerUnaryInterceptor(), echo)
			for range 2 {
				got, _, err := es.call(t.Context(), big)
				require.NoError(t, err)
				require.Equal(t, "echo:"+big, got)
			}
			require.Equal(t, int32(1), es.calls.Load())
		})
	}
}

func TestCache_CorruptEntryIsAMiss(t *testing.T) {
	t.Parallel()

	cacher := newMapCacher()
	si := grpccache.ServerInterceptor(cacher,
		grpccache.WithMethod(echoMethod, &wrapperspb.StringValue{}), grpccache.WithPublicMethods(echoMethod))
	es := startEcho(t, si.ServerUnaryInterceptor(), echo)

	_, _, err := es.call(t.Context(), "a")
	require.NoError(t, err)
	cacher.corruptAll()

	got, _, err := es.call(t.Context(), "a")
	require.NoError(t, err)
	require.Equal(t, "echo:a", got)
	require.Equal(t, int32(2), es.calls.Load(), "undecodable entry falls through to the handler")
}

func TestCache_StorageFailuresDegradeGracefully(t *testing.T) {
	t.Parallel()

	cacher := newMapCacher()
	cacher.getErr = errors.New("backend down")
	cacher.saveErr = errors.New("backend down")
	es := startEcho(t, grpccache.ServerInterceptor(cacher,
		grpccache.WithMethod(echoMethod, &wrapperspb.StringValue{}),
		grpccache.WithPublicMethods(echoMethod),
		grpccache.WithCacheDecision(grpccache.DefaultSuccessAndErrorDecision(time.Minute, time.Minute)),
	).ServerUnaryInterceptor(), func(_ context.Context, in *wrapperspb.StringValue) (any, error) {
		if in.GetValue() == "missing" {
			return nil, status.Error(codes.NotFound, "nope")
		}
		return wrapperspb.String("ok"), nil
	})

	for range 2 {
		got, _, err := es.call(t.Context(), "a")
		require.NoError(t, err)
		require.Equal(t, "ok", got)
	}
	_, _, err := es.call(t.Context(), "missing")
	require.Equal(t, codes.NotFound, status.Code(err))
	require.Equal(t, int32(3), es.calls.Load())
	require.Equal(t, 3, cacher.saves, "a save is attempted for every cacheable response")
}

func TestCache_NonProtoResponseNotCached(t *testing.T) {
	t.Parallel()

	cacher := newMapCacher()
	unary := grpccache.ServerInterceptor(cacher,
		grpccache.WithMethod(echoMethod, &wrapperspb.StringValue{}), grpccache.WithPublicMethods(echoMethod)).ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: echoMethod}

	calls := 0
	handler := func(context.Context, any) (any, error) { calls++; return "plain string", nil }
	for range 2 {
		resp, err := unary(t.Context(), wrapperspb.String("a"), info, handler)
		require.NoError(t, err)
		require.Equal(t, "plain string", resp)
	}
	require.Equal(t, 2, calls)
	require.Empty(t, cacher.keys())
}

func TestCache_MetadataKeysPartitionEntries(t *testing.T) {
	t.Parallel()

	cacher := newMapCacher()
	es := startEcho(t, grpccache.ServerInterceptor(cacher,
		grpccache.WithMethod(echoMethod, &wrapperspb.StringValue{}),
		grpccache.WithPublicMethods(echoMethod),
		grpccache.WithMetadataKeys("x-tenant"),
	).ServerUnaryInterceptor(), echo)

	for _, tenant := range []string{"t1", "t2", "t1"} {
		ctx := metadata.AppendToOutgoingContext(t.Context(), "x-tenant", tenant)
		_, _, err := es.call(ctx, "a")
		require.NoError(t, err)
	}
	require.Equal(t, int32(2), es.calls.Load(), "t1 repeat is a hit, t2 a separate entry")
	require.Len(t, cacher.keys(), 2)
}

func TestCache_MetadataProcessor(t *testing.T) {
	t.Parallel()

	var seen atomic.Int32
	cacher := newMapCacher()
	es := startEcho(t, grpccache.ServerInterceptor(cacher,
		grpccache.WithMethod(echoMethod, &wrapperspb.StringValue{}),
		grpccache.WithPublicMethods(echoMethod),
		grpccache.WithMetadataProcessor(func(_ context.Context, md metadata.MD) map[string]string {
			seen.Add(1)
			return map[string]string{"user": strings.Join(md.Get("authorization"), ",")}
		}),
	).ServerUnaryInterceptor(), echo)

	for _, user := range []string{"alice", "bob", "alice"} {
		ctx := metadata.AppendToOutgoingContext(t.Context(), "authorization", user)
		_, _, err := es.call(ctx, "a")
		require.NoError(t, err)
	}
	require.Equal(t, int32(3), seen.Load())
	require.Equal(t, int32(2), es.calls.Load())
}

func TestDefaultSerializer_UnmarshalErrors(t *testing.T) {
	t.Parallel()

	s := grpccache.NewDefaultSerializer(nil)

	_, err := s.UnmarshalForMethod(t.Context(), []byte{0x0a}, &grpccache.MethodPolicy{ResponseProto: "not proto"})
	require.Error(t, err)

	_, err = s.UnmarshalForMethod(t.Context(), []byte{0xff, 0xff}, &grpccache.MethodPolicy{ResponseProto: &wrapperspb.StringValue{}})
	require.Error(t, err)

	data, err := s.MarshalForMethod(t.Context(), wrapperspb.String("v"), &grpccache.MethodPolicy{})
	require.NoError(t, err)
	got, err := s.UnmarshalForMethod(t.Context(), data, &grpccache.MethodPolicy{ResponseProto: &wrapperspb.StringValue{}})
	require.NoError(t, err)
	require.Equal(t, "v", got.(*wrapperspb.StringValue).GetValue())
}
