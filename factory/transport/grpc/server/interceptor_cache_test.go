// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/principal"
	"github.com/altessa-s/go-atlas/cache"
	"github.com/altessa-s/go-atlas/cache/storages/freecache"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
	grpccache "github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"
)

const cacheReadMethod = "/test.Resources/GetResource"

func cacheTestBuilder() *ServerBuilder {
	return New(&grpcconfig.Config{Interceptors: &grpcconfig.Interceptors{
		Cache: &grpcconfig.CacheInterceptor{BaseInterceptor: enabled, SuccessTTL: time.Minute},
	}}).UseCacher(cache.New(freecache.New())).
		UseCacheMethodPolicies(grpccache.MethodPolicy{Name: cacheReadMethod, ResponseProto: &pb.Resource{}})
}

func callCacheChain(t *testing.T, b *ServerBuilder) func(context.Context, grpc.UnaryHandler) (any, error) {
	t.Helper()
	require.Empty(t, b.errs)
	ordered, err := interceptors.OrderServerInterceptors(b.interceptors...)
	require.NoError(t, err)
	unary := make([]grpc.UnaryServerInterceptor, 0, len(ordered))
	for _, interceptor := range ordered {
		unary = append(unary, interceptor.ServerUnaryInterceptor())
	}
	info := &grpc.UnaryServerInfo{FullMethod: cacheReadMethod}
	return func(ctx context.Context, handler grpc.UnaryHandler) (any, error) {
		for _, interceptor := range slices.Backward(unary) {
			next := handler
			handler = func(ctx context.Context, req any) (any, error) {
				return interceptor(ctx, req, info, next)
			}
		}
		return handler(ctx, wrapperspb.String("same"))
	}
}

func TestFactoryCacheRunsAfterAuthenticationAndAuthorization(t *testing.T) {
	t.Parallel()

	for _, registration := range []string{"cache first", "auth first", "bulk"} {
		t.Run(registration, func(t *testing.T) {
			t.Parallel()
			allow := true
			b := cacheTestBuilder()
			b.cfg.Interceptors.Auth = &grpcconfig.AuthInterceptor{BaseInterceptor: enabled}
			b.UseAuth(auth.AuthFunc(func(context.Context, auth.Request) (any, error) {
				return principal.Principal{Subject: "alice", Tenant: "tenant"}, nil
			}), auth.ClientAuthFunc(func(ctx context.Context, _ auth.Credentials) (context.Context, error) {
				if !allow {
					return ctx, status.Error(codes.PermissionDenied, "denied")
				}
				return ctx, nil
			}))
			switch registration {
			case "cache first":
				b.WithCacheInterceptor().WithAuthInterceptor()
			case "auth first":
				b.WithAuthInterceptor().WithCacheInterceptor()
			case "bulk":
				b.WithInterceptors()
			}
			call := callCacheChain(t, b)
			calls := 0
			handler := func(context.Context, any) (any, error) {
				calls++
				return &pb.Resource{Name: "n"}, nil
			}
			ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("authorization", "Bearer test-token"))
			for range 2 {
				_, err := call(ctx, handler)
				require.NoError(t, err)
			}
			_, err := call(t.Context(), handler)
			require.Equal(t, codes.Unauthenticated, status.Code(err), "tokenless call after cache warmup")
			allow = false
			_, err = call(ctx, handler)
			require.Equal(t, codes.PermissionDenied, status.Code(err), "authorization must run on a cache hit")
			require.Equal(t, 1, calls)

			order, err := b.dependencyOrder()
			require.NoError(t, err)
			authIndex, cacheIndex := slices.Index(order, auth.Name()), slices.Index(order, grpccache.Name())
			require.GreaterOrEqual(t, authIndex, 0)
			require.Greater(t, cacheIndex, authIndex, "%v", order)
		})
	}
}

func TestFactoryCachePartitionsReadMasks(t *testing.T) {
	t.Parallel()

	for _, header := range []string{fieldmask.DefaultMetadataReadMaskHeader, "x-read-projection"} {
		t.Run(header, func(t *testing.T) {
			t.Parallel()
			b := cacheTestBuilder()
			b.cfg.Interceptors.FieldMask = &grpcconfig.FieldMaskInterceptor{BaseInterceptor: enabled}
			b.cfg.Interceptors.Cache.KeyMetadata = []string{"x-api-version"}
			if header != fieldmask.DefaultMetadataReadMaskHeader {
				b.cfg.Interceptors.FieldMask.MetadataReadMaskHeader = header
			}
			call := callCacheChain(t, b.WithCacheInterceptor().WithFieldMaskInterceptor())
			calls := 0
			for _, path := range []string{"name", "description", "name"} {
				ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(header, path))
				got, err := call(ctx, func(context.Context, any) (any, error) {
					calls++
					return &pb.Resource{Name: "n", Description: "d"}, nil
				})
				require.NoError(t, err)
				want := &pb.Resource{Name: "n"}
				if path == "description" {
					want = &pb.Resource{Description: "d"}
				}
				require.True(t, proto.Equal(want, got.(*pb.Resource)), "mask %q: got %v", path, got)
			}
			require.Equal(t, 2, calls)
		})
	}
}

func TestFactoryCacheRequiresIdentityWhenAuthIsRegistered(t *testing.T) {
	t.Parallel()

	for _, registration := range []string{"registered after cache", "excluded"} {
		t.Run(registration, func(t *testing.T) {
			t.Parallel()
			b := cacheTestBuilder()
			ignored := enabled
			ignored.IgnoreMethods = []string{cacheReadMethod}
			b.cfg.Interceptors.Auth = &grpcconfig.AuthInterceptor{BaseInterceptor: ignored}
			b.UseAuth(auth.AuthFunc(func(context.Context, auth.Request) (any, error) {
				return principal.Principal{Subject: "alice"}, nil
			}), nil)
			wantCalls := 2
			if registration == "excluded" {
				b.WithInterceptors(auth.ID)
				wantCalls = 1
			} else {
				b.WithCacheInterceptor().WithAuthInterceptor()
			}

			call := callCacheChain(t, b)
			calls := 0
			for range 2 {
				_, err := call(t.Context(), func(context.Context, any) (any, error) {
					calls++
					return &pb.Resource{Name: "n"}, nil
				})
				require.NoError(t, err)
			}
			require.Equal(t, wantCalls, calls)
		})
	}
}
