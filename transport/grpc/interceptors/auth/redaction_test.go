// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/scope"
	"github.com/altessa-s/go-atlas/observability/tracing/adapters/console"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/logger"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/tracing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	obstracing "github.com/altessa-s/go-atlas/observability/tracing"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestAuthCallbackRedaction(t *testing.T) {
	t.Parallel()
	const bearer = "credential-grpc-47ae"
	const secret = "SECRET"
	const diagnostic = "Bearer " + bearer + " https://id.test/?api_key=" + secret
	st, err := status.New(codes.PermissionDenied, diagnostic).WithDetails(&errdetails.ErrorInfo{Reason: diagnostic})
	require.NoError(t, err)
	callbackErr := fmt.Errorf("provider: %w: %w", st.Err(), scope.ErrAccessDenied)
	for _, stage := range []string{"extract", "authenticate", "client"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			var logs, spans bytes.Buffer
			log := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			tr := obstracing.New(obstracing.WithAdapter(console.New(console.WithWriter(&spans), console.WithPrettyPrint(false))))
			t.Cleanup(func() { require.NoError(t, tr.Shutdown(context.WithoutCancel(t.Context()))) })
			extract := auth.TokenExtractorFunc(func(ctx context.Context) (string, error) {
				if stage == "extract" {
					return "", callbackErr
				}
				return auth.ExtractBearerToken().ExtractToken(ctx)
			})
			authenticate := auth.AuthFunc(func(context.Context, auth.Request) (any, error) {
				if stage == "authenticate" {
					return nil, callbackErr
				}
				return "user", nil
			})
			authorize := auth.ClientAuthFunc(func(ctx context.Context, _ auth.Credentials) (context.Context, error) { return ctx, callbackErr })
			authn := auth.ServerInterceptor(auth.WithLogger(log), auth.WithTokenExtractor(extract), auth.WithAuthFn(authenticate), auth.WithClientAuth(authorize))
			// Exercise callback redaction even with raw diagnostics enabled.
			opts, err := interceptors.NewChain(
				authn, errstatus.ServerInterceptor(errstatus.WithLogger(log), errstatus.WithLogRawErrors(true)),
				logger.ServerInterceptor(logger.Slog(log), logger.WithIgnorePatterns(regexp.MustCompile("^$")), logger.WithLogRawErrors(true)),
				tracing.ServerInterceptor(tr, tracing.WithRecordRawErrors(true)),
			).ServerOptions()
			require.NoError(t, err)
			listener := bufconn.Listen(65536)
			server := grpc.NewServer(opts...)
			healthpb.RegisterHealthServer(server, health.NewServer())
			go func() { _ = server.Serve(listener) }()
			t.Cleanup(server.Stop)
			conn, err := grpc.NewClient("passthrough:///auth-test",
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }),
			)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, conn.Close()) })
			ctx := metadata.AppendToOutgoingContext(t.Context(), "authorization", "Bearer "+bearer)
			_, err = healthpb.NewHealthClient(conn).Check(ctx, &healthpb.HealthCheckRequest{})
			require.Equal(t, codes.PermissionDenied, status.Code(err))
			require.NoError(t, tr.ForceFlush(ctx))
			require.Contains(t, logs.String(), "authenticating with token")
			require.Contains(t, logs.String(), "call finished")
			require.Contains(t, spans.String(), "rpc.grpc.status_code")
			require.Contains(t, spans.String(), "exception.message")
			for _, surface := range []string{status.Convert(err).Proto().String(), logs.String(), spans.String()} {
				require.NotContains(t, surface, bearer)
				require.NotContains(t, surface, secret)
			}
			require.Contains(t, logs.String(), "scope_denied")
			localCtx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("authorization", "Bearer "+bearer))
			_, err = authn.ServerUnaryInterceptor()(localCtx, nil, &grpc.UnaryServerInfo{FullMethod: "/test.Auth/Check"}, nil)
			require.ErrorIs(t, err, callbackErr)
			require.ErrorIs(t, err, scope.ErrAccessDenied)
		})
	}
}

func TestAuthCallbackPlainError(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		cause error
		code  codes.Code
	}{
		{"unclassified", errors.New("provider failed"), codes.Internal},
		{"canceled", context.Canceled, codes.Canceled},
		{"deadline", context.DeadlineExceeded, codes.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cb := fmt.Errorf("Bearer private-plain-token: %w", tc.cause)
			authn := auth.ServerUnaryInterceptor(auth.WithTokenExtractor(auth.TokenExtractorFunc(func(context.Context) (string, error) { return "", cb })))
			_, err := authn(t.Context(), nil, &grpc.UnaryServerInfo{FullMethod: "/test.Auth/Check"}, nil)
			require.ErrorIs(t, err, tc.cause)
			require.NotContains(t, err.Error(), "private-plain-token")
			convert := errstatus.ServerUnaryInterceptor()
			_, converted := convert(t.Context(), nil, &grpc.UnaryServerInfo{}, func(context.Context, any) (any, error) { return nil, err })
			require.Equal(t, tc.code, status.Code(converted))
		})
	}
}

func TestAuthCallbackRejection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		rejection error
		code      codes.Code
		reason    string
		domain    string
	}{
		{"reason and domain", &auth.Rejection{Code: codes.Unauthenticated, Reason: "TOKEN_REVOKED", Domain: "accounts.example.com"},
			codes.Unauthenticated, "TOKEN_REVOKED", "accounts.example.com"},
		{"wrapped", fmt.Errorf("check account: %w", &auth.Rejection{Code: codes.PermissionDenied, Reason: "ACCOUNT_SUSPENDED"}),
			codes.PermissionDenied, "ACCOUNT_SUSPENDED", ""},
		{"free text reason", &auth.Rejection{Code: codes.Unauthenticated, Reason: "Bearer private-rejection-token"}, codes.Unauthenticated, "", ""},
		{"free text domain", &auth.Rejection{Code: codes.Unauthenticated, Reason: "TOKEN_REVOKED", Domain: "Bearer private token"},
			codes.Unauthenticated, "TOKEN_REVOKED", ""},
		{"no code", &auth.Rejection{Reason: "TOKEN_REVOKED"}, codes.Unauthenticated, "TOKEN_REVOKED", ""},
		{"code above the range", &auth.Rejection{Code: codes.Code(99), Reason: "TOKEN_REVOKED"}, codes.Unauthenticated, "TOKEN_REVOKED", ""},
		{"code just above the range", &auth.Rejection{Code: codes.Unauthenticated + 1, Reason: "TOKEN_REVOKED"},
			codes.Unauthenticated, "TOKEN_REVOKED", ""},
		{"lowest code", &auth.Rejection{Code: codes.Canceled, Reason: "TOKEN_REVOKED"}, codes.Canceled, "TOKEN_REVOKED", ""},
		{"nil rejection", (*auth.Rejection)(nil), codes.Unauthenticated, "", ""},
		{"wrapped nil rejection", fmt.Errorf("check account: %w", (*auth.Rejection)(nil)), codes.Unauthenticated, "", ""},
	} {
		for _, stage := range []string{"extract", "authenticate", "client"} {
			t.Run(tc.name+"/"+stage, func(t *testing.T) {
				t.Parallel()
				extract := auth.TokenExtractorFunc(func(context.Context) (string, error) {
					if stage == "extract" {
						return "", tc.rejection
					}
					return "token", nil
				})
				authenticate := auth.AuthFunc(func(context.Context, auth.Request) (any, error) {
					if stage == "authenticate" {
						return nil, tc.rejection
					}
					return "user", nil
				})
				authorize := auth.ClientAuthFunc(func(ctx context.Context, _ auth.Credentials) (context.Context, error) {
					if stage == "client" {
						return ctx, tc.rejection
					}
					return ctx, nil
				})
				authn := auth.ServerUnaryInterceptor(auth.WithTokenExtractor(extract), auth.WithAuthFn(authenticate), auth.WithClientAuth(authorize))
				_, err := authn(t.Context(), nil, &grpc.UnaryServerInfo{FullMethod: "/test.Auth/Check"}, func(context.Context, any) (any, error) {
					t.Fatal("the handler ran after a rejection")
					return nil, nil
				})
				require.ErrorIs(t, err, tc.rejection)
				require.NotPanics(t, func() { _ = tc.rejection.Error() })
				convert := errstatus.ServerUnaryInterceptor()
				_, converted := convert(t.Context(), nil, &grpc.UnaryServerInfo{}, func(context.Context, any) (any, error) { return nil, err })
				st := status.Convert(converted)
				require.Equal(t, tc.code, st.Code())
				require.Equal(t, tc.code.String(), st.Message())
				require.NotContains(t, st.Proto().String(), "private")
				var info *errdetails.ErrorInfo
				for _, d := range st.Details() {
					if i, ok := d.(*errdetails.ErrorInfo); ok {
						info = i
					}
				}
				if tc.reason == "" {
					require.Nil(t, info)
					return
				}
				require.NotNil(t, info)
				require.Equal(t, tc.reason, info.GetReason())
				require.Equal(t, tc.domain, info.GetDomain())
			})
		}
	}
}

func TestRejectionStatus(t *testing.T) {
	t.Parallel()
	var nilRejection *auth.Rejection
	require.Equal(t, "Unauthenticated", nilRejection.Error())
	require.Equal(t, codes.Unauthenticated, nilRejection.GRPCStatus().Code())
	require.Equal(t, "PermissionDenied: ACCOUNT_SUSPENDED", (&auth.Rejection{Code: codes.PermissionDenied, Reason: "ACCOUNT_SUSPENDED"}).Error())
	require.Equal(t, codes.Unauthenticated, (&auth.Rejection{Code: codes.Code(42)}).GRPCStatus().Code())
	require.Equal(t, codes.Unauthenticated, (&auth.Rejection{Code: codes.OK}).GRPCStatus().Code())
	require.Equal(t, codes.Aborted, status.Code(&auth.Rejection{Code: codes.Aborted}))
}
