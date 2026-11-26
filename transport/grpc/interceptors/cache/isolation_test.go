// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/oidc"
	"github.com/altessa-s/go-atlas/auth/principal"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/cache"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/fieldmask"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	pb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

const readMethod = "/test.Resources/GetResource"

func TestCacheSkipsUnmarkedAnonymousMethod(t *testing.T) {
	t.Parallel()

	cacher := newMapCacher()
	unary := cache.ServerInterceptor(cacher, cache.WithMethod(echoMethod, &wrapperspb.StringValue{})).ServerUnaryInterceptor()
	for _, want := range []string{"first", "second"} {
		got, err := unary(t.Context(), wrapperspb.String("same"), &grpc.UnaryServerInfo{FullMethod: echoMethod},
			func(context.Context, any) (any, error) { return wrapperspb.String(want), nil })
		require.NoError(t, err)
		require.Equal(t, want, got.(*wrapperspb.StringValue).GetValue())
	}
	require.Zero(t, cacher.gets)
	require.Zero(t, cacher.saves)
}

func TestCachePartitionsVerifiedPrincipals(t *testing.T) {
	t.Parallel()

	cacher := newMapCacher()
	unary := cache.ServerInterceptor(cacher, cache.WithMethod(echoMethod, &wrapperspb.StringValue{})).ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: echoMethod}
	handler := func(ctx context.Context, _ any) (any, error) {
		p, _ := principal.FromContext(ctx)
		return wrapperspb.String(p.Tenant + "/" + p.Subject), nil
	}
	for _, p := range []principal.Principal{
		{Tenant: "tenant", Subject: "alice"},
		{Tenant: "tenant", Subject: "bob"},
		{Tenant: "other", Subject: "bob"},
		{Tenant: "tenant", Subject: "alice"},
	} {
		got, err := unary(principal.NewContext(t.Context(), p), wrapperspb.String("same"), info, handler)
		require.NoError(t, err)
		require.Equal(t, p.Tenant+"/"+p.Subject, got.(*wrapperspb.StringValue).GetValue())
	}
	require.Equal(t, 3, cacher.saves)
}

func TestCacheVerifiesFullTupleOnKeyCollision(t *testing.T) {
	t.Parallel()

	for _, changed := range []string{"request", "mask", "method", "principal"} {
		t.Run(changed, func(t *testing.T) {
			t.Parallel()

			const otherMethod = "/test.Resources/GetOtherResource"
			header := fieldmask.DefaultMetadataReadMaskHeader
			cacher := newMapCacher()
			unary := cache.ServerInterceptor(cacher,
				cache.WithMethod(readMethod, &wrapperspb.StringValue{}),
				cache.WithMethod(otherMethod, &wrapperspb.StringValue{}),
				cache.WithMetadataKeys(header),
				cache.WithKeyGenerator(func(context.Context, string, any) (string, error) { return "same-key", nil }),
			).ServerUnaryInterceptor()
			call := func(method, subject, req, mask, want string) {
				ctx := principal.NewContext(t.Context(), principal.Principal{Subject: subject})
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(header, mask))
				got, err := unary(ctx, wrapperspb.String(req), &grpc.UnaryServerInfo{FullMethod: method},
					func(context.Context, any) (any, error) { return wrapperspb.String(want), nil })
				require.NoError(t, err)
				require.Equal(t, want, got.(*wrapperspb.StringValue).GetValue())
			}

			call(readMethod, "alice", "one", "name", "first")
			switch changed {
			case "request":
				call(readMethod, "alice", "two", "name", "second")
			case "mask":
				call(readMethod, "alice", "one", "description", "second")
			case "method":
				call(otherMethod, "alice", "one", "name", "second")
			case "principal":
				call(readMethod, "bob", "one", "name", "second")
			}
			require.Len(t, cacher.keys(), 1)
			require.Equal(t, 2, cacher.saves)
		})
	}
}

func TestCachePartitionsDefaultReadMasks(t *testing.T) {
	t.Parallel()

	cacher := newMapCacher()
	unary := cache.ServerInterceptor(cacher,
		cache.WithMethod(readMethod, &pb.Resource{}), cache.WithPublicMethods(readMethod)).ServerUnaryInterceptor()
	mask := fieldmask.ServerInterceptor().ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: readMethod}
	for _, path := range []string{"name", "description", "name"} {
		ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs(fieldmask.DefaultMetadataReadMaskHeader, path))
		got, err := unary(ctx, wrapperspb.String("same"), info, func(ctx context.Context, req any) (any, error) {
			return mask(ctx, req, info, func(context.Context, any) (any, error) {
				return &pb.Resource{Name: "n", Description: "d"}, nil
			})
		})
		require.NoError(t, err)
		want := &pb.Resource{Name: "n"}
		if path == "description" {
			want = &pb.Resource{Description: "d"}
		}
		require.True(t, proto.Equal(want, got.(*pb.Resource)), "mask %q: got %v", path, got)
	}
	require.Equal(t, 2, cacher.saves)
}

func TestCachePreservesStatusDetails(t *testing.T) {
	t.Parallel()

	notFound, err := status.New(codes.NotFound, "missing resource").WithDetails(&errdetails.ResourceInfo{
		ResourceType: "test.Resource", ResourceName: "resources/1",
	})
	require.NoError(t, err)
	si := cache.ServerInterceptor(newMapCacher(),
		cache.WithMethod(echoMethod, &wrapperspb.StringValue{}),
		cache.WithPublicMethods(echoMethod),
		cache.WithCacheDecision(cache.DefaultSuccessAndErrorDecision(time.Minute, time.Minute)))
	es := startEcho(t, si.ServerUnaryInterceptor(), func(context.Context, *wrapperspb.StringValue) (any, error) {
		return nil, notFound.Err()
	})

	_, _, miss := es.call(t.Context(), "same")
	_, _, hit := es.call(t.Context(), "same")
	require.True(t, proto.Equal(notFound.Proto(), status.Convert(miss).Proto()))
	require.True(t, proto.Equal(status.Convert(miss).Proto(), status.Convert(hit).Proto()), "cached status: %v", status.Convert(hit).Proto())
	require.Equal(t, int32(1), es.calls.Load())
}

func TestCacheBypassesUnidentifiedCalls(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"empty subject", "custom credential", "required auth absent", "empty custom namespace"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cacher := newMapCacher()
			info := &grpc.UnaryServerInfo{FullMethod: echoMethod}
			ctx := t.Context()
			opts := []cache.Option{cache.WithMethod(echoMethod, &wrapperspb.StringValue{}), cache.WithPublicMethods(echoMethod)}
			switch name {
			case "empty subject":
				ctx = principal.NewContext(ctx, principal.Principal{Tenant: "tenant"})
			case "required auth absent":
				opts = append(opts, cache.WithAuthRequired())
			case "empty custom namespace":
				opts = append(opts, cache.WithIdentityFunc(func(context.Context) (string, cache.IdentityState) {
					return "", cache.Verified
				}))
			}

			handler := func(context.Context, any) (any, error) { return wrapperspb.String("fresh"), nil }
			unary := cache.ServerInterceptor(cacher, opts...).ServerUnaryInterceptor()
			if name == "custom credential" {
				customAuth := auth.ServerInterceptor(auth.WithAuthFn(auth.AuthFunc(func(context.Context, auth.Request) (any, error) {
					return struct{ ID string }{ID: "alice"}, nil
				}))).ServerUnaryInterceptor()
				cacheUnary := unary
				unary = func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
					return customAuth(ctx, req, info, func(ctx context.Context, req any) (any, error) {
						return cacheUnary(ctx, req, info, next)
					})
				}
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer test-token"))
			}
			for range 2 {
				got, err := unary(ctx, wrapperspb.String("same"), info, handler)
				require.NoError(t, err)
				require.Equal(t, "fresh", got.(*wrapperspb.StringValue).GetValue())
			}
			require.Zero(t, cacher.gets)
			require.Zero(t, cacher.saves)
		})
	}
}

func TestCacheUsesCustomVerifiedNamespace(t *testing.T) {
	t.Parallel()

	cacher := newMapCacher()
	type issuerKey struct{}
	unary := cache.ServerInterceptor(cacher,
		cache.WithMethod(echoMethod, &wrapperspb.StringValue{}),
		cache.WithAuthRequired(),
		cache.WithIdentityFunc(func(ctx context.Context) (string, cache.IdentityState) {
			issuer, _ := ctx.Value(issuerKey{}).(string)
			return issuer, cache.Verified
		}),
	).ServerUnaryInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: echoMethod}
	calls := 0
	for _, issuer := range []string{"issuer1", "issuer2", "issuer1"} {
		ctx := context.WithValue(t.Context(), issuerKey{}, issuer)
		got, err := unary(ctx, wrapperspb.String("same"), info, func(ctx context.Context, _ any) (any, error) {
			calls++
			return wrapperspb.String(issuer), nil
		})
		require.NoError(t, err)
		require.Equal(t, issuer, got.(*wrapperspb.StringValue).GetValue())
	}
	require.Equal(t, 2, calls)
}

func TestCacheBypassesCustomCredentialsWithInheritedPrincipal(t *testing.T) {
	t.Parallel()

	cacher := newMapCacher()
	unary := cache.ServerInterceptor(cacher,
		cache.WithMethod(echoMethod, &wrapperspb.StringValue{}), cache.WithAuthRequired(),
	).ServerUnaryInterceptor()
	ctx := principal.NewContext(t.Context(), principal.Principal{Subject: "upstream"})
	for _, user := range []string{"alice", "bob"} {
		got := callWithCacheCredentials(t, ctx, unary, struct{ ID string }{user}, user)
		require.Equal(t, user, got)
	}
	require.Zero(t, cacher.gets)
	require.Zero(t, cacher.saves)
}

func TestCachePartitionsGrantsAndIssuer(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name          string
		first, second *oidc.Claims
	}{
		{name: "scopes", first: &oidc.Claims{Scopes: []string{"read", "write"}}, second: &oidc.Claims{Scopes: []string{"read"}}},
		{name: "roles", first: &oidc.Claims{RawClaims: map[string]any{"roles": []string{"admin"}}}, second: &oidc.Claims{}},
		{name: "issuer", first: &oidc.Claims{Issuer: "issuer-a"}, second: &oidc.Claims{Issuer: "issuer-b"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.first.Subject, tc.second.Subject = "alice", "alice"
			cacher := newMapCacher()
			unary := cache.ServerInterceptor(cacher, cache.WithMethod(echoMethod, &wrapperspb.StringValue{})).ServerUnaryInterceptor()

			require.Equal(t, "first", callWithCacheCredentials(t, t.Context(), unary, tc.first, "first"))
			require.Equal(t, "second", callWithCacheCredentials(t, t.Context(), unary, tc.second, "second"))
			require.Equal(t, "first", callWithCacheCredentials(t, t.Context(), unary, tc.first, "unexpected miss"))
			require.Equal(t, 2, cacher.saves)
		})
	}

	t.Run("grant order", func(t *testing.T) {
		t.Parallel()
		cacher := newMapCacher()
		unary := cache.ServerInterceptor(cacher, cache.WithMethod(echoMethod, &wrapperspb.StringValue{})).ServerUnaryInterceptor()
		first := principal.Principal{Subject: "alice", Scopes: []string{"write", "read"}, Roles: []string{"owner", "admin"}}
		second := principal.Principal{Subject: "alice", Scopes: []string{"read", "write"}, Roles: []string{"admin", "owner"}}
		require.Equal(t, "first", callWithCacheCredentials(t, t.Context(), unary, first, "first"))
		require.Equal(t, "first", callWithCacheCredentials(t, t.Context(), unary, second, "unexpected miss"))
		require.Equal(t, []string{"write", "read"}, first.Scopes)
		require.Equal(t, []string{"owner", "admin"}, first.Roles)
		require.Equal(t, 1, cacher.saves)
	})
}

func callWithCacheCredentials(t *testing.T, ctx context.Context, unary grpc.UnaryServerInterceptor, data any, response string) string {
	t.Helper()

	authUnary := auth.ServerInterceptor(auth.WithAuthFn(auth.AuthFunc(func(context.Context, auth.Request) (any, error) {
		return data, nil
	}))).ServerUnaryInterceptor()
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer test-token"))
	info := &grpc.UnaryServerInfo{FullMethod: echoMethod}
	got, err := authUnary(ctx, wrapperspb.String("same"), info, func(ctx context.Context, req any) (any, error) {
		return unary(ctx, req, info, func(context.Context, any) (any, error) {
			return wrapperspb.String(response), nil
		})
	})
	require.NoError(t, err)
	return got.(*wrapperspb.StringValue).GetValue()
}
