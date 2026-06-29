// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"errors"

	"github.com/altessa-s/go-atlas/auth/scope"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrPrincipalTypeMismatch reports that the value the [Auth] function stored in
// [Credentials.Data] is not of the principal type P the [scope.Enforcer] was
// built for: the Auth function and the enforcer disagree on the principal type.
// This is a server-side wiring bug, not a client permission problem.
//
// [ScopeClientAuth] fails closed on it — the call is still denied — but folds it
// into a codes.PermissionDenied error indistinguishable to the client from a
// genuine denial, so the misconfiguration never leaks. It stays observable to
// operators through errors.Is and the auth interceptor's warning log.
var ErrPrincipalTypeMismatch = errors.New("scope: principal type mismatch between authFn result and enforcer")

// errScopeDenied is the gRPC-shaped denial [ScopeClientAuth] returns when the
// enforcer denies an authenticated principal. It carries codes.PermissionDenied
// (so the auth interceptor surfaces it verbatim instead of rewriting it to
// Unauthenticated) and wraps [scope.ErrAccessDenied] so callers can still match
// it with errors.Is.
var errScopeDenied = interceptors.NewError(
	status.New(codes.PermissionDenied, "permission denied"),
	scope.ErrAccessDenied,
)

// errPrincipalTypeMismatch is the gRPC-shaped, fail-closed error returned when
// the principal type assertion fails. Outwardly it mirrors errScopeDenied —
// codes.PermissionDenied with the same generic message, so the client cannot
// tell a wiring bug from a real denial — but it wraps both [scope.ErrAccessDenied]
// (so deny-matching keeps working) and [ErrPrincipalTypeMismatch] (so the
// distinct cause stays visible to errors.Is and in the interceptor's log).
var errPrincipalTypeMismatch = interceptors.NewError(
	status.New(codes.PermissionDenied, "permission denied"),
	errors.Join(scope.ErrAccessDenied, ErrPrincipalTypeMismatch),
)

// ScopeClientAuth adapts a [scope.Enforcer] to the [ClientAuth] seam, enforcing
// the method→scope policy on every authenticated call. The verified principal of
// type P is read from [Credentials.Data] (the value returned by the [Auth]
// function) and the action key is [Credentials.FullyMethodName].
//
// Both failure modes fail closed with a codes.PermissionDenied error carrying an
// identical client-facing message, so neither is distinguishable to the caller:
//
//   - the enforcer denies the principal — wraps [scope.ErrAccessDenied];
//   - [Credentials.Data] is not of type P — a server-side wiring bug that
//     additionally wraps [ErrPrincipalTypeMismatch], letting operators tell it
//     apart from a real denial in the auth interceptor's warning log.
//
// On success the context is returned unchanged — the principal already travels
// in Credentials.Data, so storing it elsewhere is the caller's concern.
//
// Wire it via [WithClientAuth]:
//
//	enf := scope.NewEnforcer(registry, authorize)
//	auth.ServerInterceptor(
//	    auth.WithAuthFunc(authFunc),
//	    auth.WithClientAuth(auth.ScopeClientAuth(enf)),
//	)
func ScopeClientAuth[P any](e *scope.Enforcer[P]) ClientAuth {
	return ClientAuthFunc(func(ctx context.Context, cred Credentials) (context.Context, error) {
		p, ok := cred.Data.(P)
		if !ok {
			return ctx, errPrincipalTypeMismatch
		}
		if err := e.Enforce(p, cred.FullyMethodName); err != nil {
			return ctx, errScopeDenied
		}
		return ctx, nil
	})
}
