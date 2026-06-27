// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"

	"github.com/altessa-s/go-atlas/auth/scope"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// errScopeDenied is the gRPC-shaped denial returned by [ScopeClientAuth]. It
// carries codes.PermissionDenied (so the auth interceptor surfaces it verbatim
// instead of rewriting it to Unauthenticated) and wraps [scope.ErrAccessDenied]
// so callers can still match it with errors.Is.
var errScopeDenied = interceptors.NewError(
	status.New(codes.PermissionDenied, "permission denied"),
	scope.ErrAccessDenied,
)

// ScopeClientAuth adapts a [scope.Enforcer] to the [ClientAuth] seam, enforcing
// the method→scope policy on every authenticated call. The verified principal of
// type P is read from [Credentials.Data] (the value returned by the [Auth]
// function) and the action key is [Credentials.FullyMethodName].
//
// A principal of the wrong type, or one the enforcer denies, yields a
// codes.PermissionDenied error. On success the context is returned unchanged —
// the principal already travels in Credentials.Data, so storing it elsewhere is
// the caller's concern.
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
			return ctx, errScopeDenied
		}
		if err := e.Enforce(p, cred.FullyMethodName); err != nil {
			return ctx, errScopeDenied
		}
		return ctx, nil
	})
}
