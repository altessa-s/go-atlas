// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"strings"
	"time"

	"github.com/altessa-s/go-atlas/core/types/redacted"
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
	method := i.InternMethod(callMeta.FullyMethodName)

	if i.ShouldIgnore(method) {
		i.LogIgnored(ctx, method)
		return ctx, nil
	}

	i.LogDebug(ctx, "authenticating with token", method)

	return i.authenticateWithToken(ctx, callMeta)
}

func (i *interceptor) authenticateWithToken(ctx context.Context, callMeta *sharedmetadata.CallMetadata) (context.Context, error) {
	method := i.InternMethod(callMeta.FullyMethodName)

	token, err := i.opts.tokenExtractor.ExtractToken(ctx)
	if err != nil {
		return ctx, err
	} else if token == "" {
		return ctx, status.Error(codes.Unauthenticated, "Missing or invalid Bearer token")
	}

	credsReq := Request{
		Base: Base{
			AuthMethod:      MethodToken,
			AuthenticatedAt: time.Now(),
			UserAgent:       UserAgentFromCallMeta(callMeta),
			ServiceName:     callMeta.ServiceName,
			MethodName:      callMeta.MethodName,
			FullyMethodName: callMeta.FullyMethodName,
		},
		Payload: &TokenCredentials{Token: redacted.RedactedString(token)},
	}

	var data any

	if i.opts.authFn != nil {
		data, err = i.opts.authFn.Auth(ctx, credsReq)
	} else {
		i.LogWarn(ctx, "no authentication function configured", method, nil)
		return ctx, status.Error(codes.Unauthenticated, "Unauthenticated")
	}

	if err != nil {
		i.LogWarn(ctx, "authentication failed", method, err)

		if _, ok := status.FromError(err); !ok {
			err = interceptors.NewError(status.New(codes.Unauthenticated, "Unauthenticated"), err)
		}
		return ctx, err
	}

	creds := Credentials{Base: credsReq.Base, Data: data}

	md, ok := grpcmetadata.FromIncomingContext(ctx)
	if ok {
		creds.Headers = make(map[string]string, len(md))
		for k, v := range md {
			if isSensitiveHeader(k) {
				continue
			}
			creds.Headers[k] = strings.Join(v, ",")
		}
	}

	var newCtx = ctx
	if i.opts.clientAuth != nil {
		newCtx, err = i.opts.clientAuth.ClientAuth(ctx, creds)
		if err != nil {
			i.LogWarn(ctx, "authentication failed", method, err)
			if _, ok := status.FromError(err); !ok {
				err = interceptors.NewError(status.New(codes.Unauthenticated, "Unauthenticated"), err)
			}
			return ctx, err
		}
	}

	i.LogDebug(ctx, "authentication successful", method)

	return newCtx, nil
}

// isSensitiveHeader reports whether a metadata key should be excluded from
// Credentials.Headers to prevent accidental leakage of secrets through
// logging, forwarding, or storage.
func isSensitiveHeader(key string) bool {
	return strings.EqualFold(key, "authorization")
}
