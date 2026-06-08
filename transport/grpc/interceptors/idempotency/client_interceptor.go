// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate --type=clientOptions --option-type=ClientOption --option-prefix=Client --output=client_options_gen.go

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// operationContextKey is the private key under which [WithOperation] stores the
// operation seed used by [UnaryClientInterceptor].
type operationContextKey struct{}

// WithOperation returns a copy of ctx tagged with operationID. Any outbound gRPC
// call made with the resulting context — directly or transitively — will be
// stamped with a deterministic [DeriveKey]-derived Idempotency-Key by
// [UnaryClientInterceptor], so retries of the same operationID collapse on the
// server. An empty operationID is treated as "unset" and disables stamping.
//
// WithOperation is safe for concurrent use; it does not mutate ctx.
func WithOperation(ctx context.Context, operationID string) context.Context {
	return context.WithValue(ctx, operationContextKey{}, operationID)
}

// OperationFromContext returns the operation seed previously attached by
// [WithOperation], if any. ok is false when no seed is set or the seed is empty.
func OperationFromContext(ctx context.Context) (operationID string, ok bool) {
	v, _ := ctx.Value(operationContextKey{}).(string)
	if v == "" {
		return "", false
	}
	return v, true
}

// SeedExtractor pulls a deterministic per-operation seed from ctx. Returning ok
// = false makes [UnaryClientInterceptor] forward the call without stamping a
// key. The default extractor delegates to [OperationFromContext].
type SeedExtractor func(ctx context.Context) (seed string, ok bool)

// MethodFilter decides whether [UnaryClientInterceptor] should consider stamping
// a key for the given fully-qualified gRPC method (e.g. "/svc.v1.Foo/Bar").
// Returning false skips the call entirely. The default filter admits every
// method — call sites already opt in implicitly by populating the context with
// [WithOperation], so list-based filtering is opt-in.
type MethodFilter func(method string) bool

// DefaultClientSeedExtractor is the [SeedExtractor] used when none is supplied
// via [WithClientSeedExtractor]. It reads the value attached by [WithOperation].
var DefaultClientSeedExtractor SeedExtractor = OperationFromContext

// DefaultClientMethodFilter is the [MethodFilter] used when none is supplied via
// [WithClientMethodFilter]. It admits every method.
var DefaultClientMethodFilter MethodFilter = func(string) bool { return true }

// clientOptions configures [UnaryClientInterceptor]. Surface is intentionally
// small; expand only when a real call site demands it. Fields are populated by
// the optgen-generated With* options in client_options_gen.go.
type clientOptions struct {
	idempotencyKeyHeader string        `optgen:"default=DefaultIdempotencyKeyHeader"`
	seedExtractor        SeedExtractor `optgen:"default=DefaultClientSeedExtractor"`
	methodFilter         MethodFilter  `optgen:"default=DefaultClientMethodFilter"`
}

// UnaryClientInterceptor returns a [grpc.UnaryClientInterceptor] that stamps a
// deterministic [DeriveKey] result onto each outbound call whose context
// carries an operation seed (see [WithOperation]).
//
// The interceptor preserves any Idempotency-Key already present on the
// outgoing context — explicit attachment via [WithKey] / [WithDerivedKey] wins
// over the seed-based path. Calls without a seed are forwarded untouched.
//
// Typical wiring:
//
//	conn, err := grpc.NewClient(target,
//	    grpc.WithTransportCredentials(creds),
//	    grpc.WithUnaryInterceptor(idempotency.UnaryClientInterceptor()),
//	)
//	...
//	ctx = idempotency.WithOperation(ctx, operationID)
//	resp, err := client.Update(ctx, req)
//
// The returned interceptor is safe for concurrent use.
func UnaryClientInterceptor(opts ...ClientOption) grpc.UnaryClientInterceptor {
	cfg := newClientOptions(opts...)

	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		callOpts ...grpc.CallOption,
	) error {
		ctx = maybeStampIdempotencyKey(ctx, method, cfg)
		return invoker(ctx, method, req, reply, cc, callOpts...)
	}
}

// maybeStampIdempotencyKey attaches a derived key to ctx when cfg's filter
// admits method, an operation seed is available, and no Idempotency-Key header
// is already set. Otherwise it returns ctx unchanged.
func maybeStampIdempotencyKey(ctx context.Context, method string, cfg *clientOptions) context.Context {
	if !cfg.methodFilter(method) {
		return ctx
	}
	seed, ok := cfg.seedExtractor(ctx)
	if !ok {
		return ctx
	}
	if md, mdOK := metadata.FromOutgoingContext(ctx); mdOK && len(md.Get(cfg.idempotencyKeyHeader)) > 0 {
		return ctx
	}
	return withHeaderValue(ctx, cfg.idempotencyKeyHeader, DeriveKey(seed, method))
}

// withHeaderValue is the header-name-agnostic twin of [WithKey]. It exists so
// the interceptor can honour a custom header without duplicating
// metadata-copy logic.
func withHeaderValue(ctx context.Context, header, value string) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		md = md.Copy()
	} else {
		md = metadata.New(nil)
	}
	md.Set(header, value)
	return metadata.NewOutgoingContext(ctx, md)
}
