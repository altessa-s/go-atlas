// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas-core/text/strings/intern"
	"github.com/altessa-s/go-atlas-core/types/redacted"
	"github.com/altessa-s/go-atlas/auth/scope"
	"github.com/altessa-s/go-atlas/internal/diagnostics"
	"github.com/altessa-s/go-atlas/internal/transport/authprincipal"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/errstatus"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sharedmetadata "github.com/altessa-s/go-atlas/transport/grpc/interceptors/metadata"
	stdGrpc "google.golang.org/grpc"
	grpcmetadata "google.golang.org/grpc/metadata"
)

const interceptorName = "auth"

// Name returns the interceptor name used for dependency resolution and chain ordering.
func Name() string { return interceptorName }

// ID is a lightweight [interceptors.Interceptor] reference for this package,
// suitable for passing to exclusion lists.
var ID = interceptors.Ref(interceptorName)

var _ interceptors.ServerInterceptor = (*interceptor)(nil)

type interceptor struct {
	interceptors.BaseInterceptor
	opts *options
}

// Dependencies returns interceptors that auth requires to run before it.
// Auth uses metadata for call information extraction and errstatus must
// wrap auth so that authentication errors are enriched with RequestInfo.
func (i *interceptor) Dependencies() []string {
	return []string{sharedmetadata.Name(), errstatus.Name()}
}

// ServerInterceptor returns a new interceptor that authenticates the request.
func ServerInterceptor(opt ...Option) interceptors.ServerInterceptor {
	opts := newOptions(opt...)

	// Set default token extractor if not configured
	if opts.tokenExtractor == nil {
		opts.tokenExtractor = ExtractBearerToken()
	}

	return &interceptor{
		BaseInterceptor: interceptors.NewBaseInterceptorWithFilter(
			interceptorName,
			opts.ignoreMethods,
			opts.ignorePatterns,
			opts.logger,
		),
		opts: opts,
	}
}

// ServerUnaryInterceptor returns a new unary server interceptor that authenticates the request.
func ServerUnaryInterceptor(opt ...Option) stdGrpc.UnaryServerInterceptor {
	return ServerInterceptor(opt...).ServerUnaryInterceptor()
}

// ServerStreamInterceptor returns a new streaming server interceptor that authenticates the request.
func ServerStreamInterceptor(opt ...Option) stdGrpc.StreamServerInterceptor {
	return ServerInterceptor(opt...).ServerStreamInterceptor()
}

// ServerUnaryInterceptor returns a new unary server interceptor that authenticates the request.
func (i *interceptor) ServerUnaryInterceptor() stdGrpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *stdGrpc.UnaryServerInfo, handler stdGrpc.UnaryHandler) (any, error) {
		ctx, meta := sharedmetadata.EnsureInContext(ctx, info.FullMethod, info)

		newCtx, err := i.auth(ctx, meta)
		if err != nil {
			return nil, err
		}
		return handler(newCtx, req)
	}
}

// ServerStreamInterceptor returns a new streaming server interceptor that authenticates the request.
func (i *interceptor) ServerStreamInterceptor() stdGrpc.StreamServerInterceptor {
	return func(srv any, stream stdGrpc.ServerStream, info *stdGrpc.StreamServerInfo, handler stdGrpc.StreamHandler) error {
		ctx, meta := sharedmetadata.EnsureInContext(stream.Context(), info.FullMethod, info)

		newCtx, err := i.auth(ctx, meta)
		if err != nil {
			return err
		}
		return handler(srv, interceptors.NewServerWrappedStream(newCtx, stream, nil))
	}
}

func (i *interceptor) auth(ctx context.Context, callMeta *sharedmetadata.CallMetadata) (context.Context, error) {
	method := intern.String(callMeta.FullyMethodName)

	if i.ShouldIgnore(method) {
		i.LogIgnored(ctx, method)
		return ctx, nil
	}

	i.LogDebug(ctx, "authenticating with token", method)

	return i.authenticateWithToken(ctx, callMeta)
}

func (i *interceptor) authenticateWithToken(ctx context.Context, callMeta *sharedmetadata.CallMetadata) (context.Context, error) {
	method := intern.String(callMeta.FullyMethodName)

	token, err := i.opts.tokenExtractor.ExtractToken(ctx)
	if err != nil {
		return ctx, i.callbackError(ctx, method, "token extraction failed", err, nil)
	} else if token == "" {
		return ctx, status.Error(codes.Unauthenticated, "Missing or invalid Bearer token")
	}

	credsReq := Request{
		AuthMethod:      MethodToken,
		AuthenticatedAt: time.Now(),
		UserAgent:       UserAgentFromCallMeta(callMeta),
		ServiceName:     callMeta.ServiceName,
		MethodName:      callMeta.MethodName,
		FullyMethodName: callMeta.FullyMethodName,
		Payload:         &TokenCredentials{Token: redacted.RedactedString(token)},
	}

	var data any

	if i.opts.authFn != nil {
		data, err = i.opts.authFn.Auth(ctx, credsReq)
	} else {
		i.LogWarn(ctx, "no authentication function configured", method, nil)
		return ctx, status.Error(codes.Unauthenticated, "Unauthenticated")
	}

	if err != nil {
		return ctx, i.callbackError(ctx, method, "authentication failed", err, statusUnauthenticated)
	}

	// The credentials always reach the handler through CredentialsFromContext.
	// Headers is a copy of the incoming metadata that only ClientAuth needs,
	// so the copy is made only when ClientAuth is configured.
	creds := Credentials{Base: credsReq.Base, Data: data}
	if i.opts.clientAuth != nil {
		creds.Headers = credentialHeaders(ctx)
	}
	newCtx := context.WithValue(ctx, credentialsKey{}, creds)
	newCtx = authprincipal.Bridge(newCtx, data)

	if i.opts.clientAuth != nil {
		newCtx, err = i.opts.clientAuth.ClientAuth(newCtx, creds)
		if err != nil {
			return ctx, i.callbackError(ctx, method, "client authorization failed", err, statusUnauthenticated)
		}
	}

	i.LogDebug(ctx, "authentication successful", method)

	return newCtx, nil
}

var statusUnauthenticated = status.New(codes.Unauthenticated, codes.Unauthenticated.String())

type grpcStatusError interface {
	error
	GRPCStatus() *status.Status
}

// callbackError rebuilds the status so neither its message, details nor the
// cause saved by callstatus can carry callback text.
func (i *interceptor) callbackError(ctx context.Context, method, operation string, err error, st *status.Status) error {
	if carrier, ok := errors.AsType[grpcStatusError](err); ok {
		st = statusUnauthenticated
		if original := carrier.GRPCStatus(); original != nil && original.Code() > codes.OK && original.Code() <= codes.Unauthenticated {
			st = status.New(original.Code(), original.Code().String())
		}
	}
	class := "token_extraction_failed"
	if st != nil {
		class = st.Code().String()
	}
	switch {
	case errors.Is(err, ErrPrincipalTypeMismatch):
		class = "principal_type_mismatch"
	case errors.Is(err, scope.ErrAccessDenied):
		class = "scope_denied"
	}
	i.LogWarn(ctx, operation, method, nil, slog.String("classification", class))
	safe := diagnostics.ClassifiedError(class, err)
	if st == nil {
		// A plain extractor error is mapped by the outer errstatus converters.
		return safe
	}
	return interceptors.NewError(st, safe)
}

// credentialHeaders copies the incoming metadata into a header map, dropping
// the keys [isSensitiveHeader] reports. It returns nil when ctx carries no
// incoming metadata.
func credentialHeaders(ctx context.Context) map[string]string {
	md, ok := grpcmetadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}
	headers := make(map[string]string, len(md))
	for k, v := range md {
		if isSensitiveHeader(k) {
			continue
		}
		headers[k] = strings.Join(v, ",")
	}
	return headers
}

// isSensitiveHeader reports whether a metadata key should be excluded from
// Credentials.Headers to prevent accidental leakage of secrets through
// logging, forwarding, or storage.
func isSensitiveHeader(key string) bool {
	return strings.EqualFold(key, "authorization") ||
		strings.EqualFold(key, "cookie") ||
		strings.EqualFold(key, "proxy-authorization")
}
