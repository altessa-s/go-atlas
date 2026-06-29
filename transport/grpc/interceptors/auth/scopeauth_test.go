// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/scope"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testPrincipal struct {
	scopes []string
}

const methodWrite = "/files.v1.Files/Write"

func newScopeAuth(t *testing.T) auth.ClientAuth {
	t.Helper()
	reg := scope.NewRegistry()
	reg.Register(methodWrite, "files:write")
	reg.Freeze()
	enf := scope.NewEnforcer(reg, scope.ScopeAuthorizer(
		func(p *testPrincipal) []string { return p.scopes },
		scope.Exact(),
	))
	return auth.ScopeClientAuth(enf)
}

func credsFor(method string, data any) auth.Credentials {
	return auth.Credentials{Base: auth.Base{FullyMethodName: method}, Data: data}
}

func TestScopeClientAuthAllows(t *testing.T) {
	t.Parallel()
	ca := newScopeAuth(t)
	_, err := ca.ClientAuth(t.Context(), credsFor(methodWrite, &testPrincipal{scopes: []string{"files:write"}}))
	require.NoError(t, err)
}

func TestScopeClientAuthDeniesMissingScope(t *testing.T) {
	t.Parallel()
	ca := newScopeAuth(t)
	_, err := ca.ClientAuth(t.Context(), credsFor(methodWrite, &testPrincipal{scopes: []string{"files:read"}}))
	require.ErrorIs(t, err, scope.ErrAccessDenied)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	// A genuine denial is not a wiring bug.
	require.NotErrorIs(t, err, auth.ErrPrincipalTypeMismatch)
}

func TestScopeClientAuthDeniesUnregistered(t *testing.T) {
	t.Parallel()
	ca := newScopeAuth(t)
	_, err := ca.ClientAuth(t.Context(), credsFor("/files.v1.Files/Unknown", &testPrincipal{scopes: []string{"files:write"}}))
	require.ErrorIs(t, err, scope.ErrAccessDenied)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestScopeClientAuthDeniesWrongPrincipalType(t *testing.T) {
	t.Parallel()
	ca := newScopeAuth(t)
	_, err := ca.ClientAuth(t.Context(), credsFor(methodWrite, "not-a-principal"))
	require.ErrorIs(t, err, scope.ErrAccessDenied)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	// A wiring bug stays observable as its own cause, yet is indistinguishable
	// to the client (same PermissionDenied status and message as a real denial).
	require.ErrorIs(t, err, auth.ErrPrincipalTypeMismatch)
	require.Equal(t, "permission denied", status.Convert(err).Message())
}
