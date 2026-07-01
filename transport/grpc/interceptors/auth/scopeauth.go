// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"errors"
	"time"

	"github.com/altessa-s/go-atlas/auth/audit"
	"github.com/altessa-s/go-atlas/auth/principal"
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
//
// Pass [WithScopeAudit] to record every decision through an
// [github.com/altessa-s/go-atlas/auth/audit.Recorder].
func ScopeClientAuth[P any](e *scope.Enforcer[P], opts ...ScopeClientAuthOption[P]) ClientAuth {
	var cfg scopeClientAuthConfig[P]
	for _, opt := range opts {
		opt(&cfg)
	}
	return ClientAuthFunc(func(ctx context.Context, cred Credentials) (context.Context, error) {
		p, ok := cred.Data.(P)
		if !ok {
			return ctx, cfg.finalize(ctx, cred, "", false, "principal_type_mismatch", errPrincipalTypeMismatch)
		}
		if err := e.Enforce(p, cred.FullyMethodName); err != nil {
			return ctx, cfg.finalize(ctx, cred, cfg.subject(p), false, "scope_denied", errScopeDenied)
		}
		return ctx, cfg.finalize(ctx, cred, cfg.subject(p), true, "", nil)
	})
}

// ScopeClientAuthPrincipal is [ScopeClientAuth] specialized to the canonical
// [principal.Principal], the standard authorization subject an authentication
// adapter stashes in Credentials.Data. It is a convenience so callers write
// ScopeClientAuthPrincipal(e) instead of ScopeClientAuth[principal.Principal](e);
// pass [WithScopeAudit] with subjectOf func(principal.Principal) string to record
// the subject (typically p.Subject).
func ScopeClientAuthPrincipal(
	e *scope.Enforcer[principal.Principal], opts ...ScopeClientAuthOption[principal.Principal],
) ClientAuth {
	return ScopeClientAuth(e, opts...)
}

// ScopeClientAuthOption configures [ScopeClientAuth].
type ScopeClientAuthOption[P any] func(*scopeClientAuthConfig[P])

type scopeClientAuthConfig[P any] struct {
	recorder  *audit.Recorder
	subjectOf func(P) string
}

// WithScopeAudit records every authorization decision through rec, keyed on the
// gRPC full method. subjectOf extracts the principal identity for the record;
// pass nil to leave the subject empty (it is not called on a principal-type
// mismatch, where no principal is available). When rec is configured with
// audit.FailureRequired and recording an otherwise-allowed call fails, the call
// is failed with codes.Internal so nothing proceeds unrecorded.
func WithScopeAudit[P any](rec *audit.Recorder, subjectOf func(P) string) ScopeClientAuthOption[P] {
	return func(c *scopeClientAuthConfig[P]) {
		c.recorder = rec
		c.subjectOf = subjectOf
	}
}

func (c scopeClientAuthConfig[P]) subject(p P) string {
	if c.subjectOf == nil {
		return ""
	}
	return c.subjectOf(p)
}

// finalize records the decision when auditing is configured and returns the
// authorization error to surface. A failed required audit on an allowed call
// becomes a codes.Internal error so the call does not proceed unrecorded.
func (c scopeClientAuthConfig[P]) finalize(
	ctx context.Context, cred Credentials, subject string, allowed bool, reason string, authErr error,
) error {
	if c.recorder == nil {
		return authErr
	}
	recErr := c.recorder.Record(ctx, audit.Decision{
		Time:       time.Now().UTC(),
		Allowed:    allowed,
		Subject:    subject,
		Action:     cred.FullyMethodName,
		Reason:     reason,
		Attributes: map[string]string{"transport": "grpc"},
	})
	if recErr != nil && authErr == nil {
		return status.Error(codes.Internal, "authorization audit failed")
	}
	return authErr
}
