// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/audit/decision"
	"github.com/altessa-s/go-atlas/auth/scope"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ auth.DecisionRecorder = (*decision.Recorder)(nil)

type decisionRecorderFunc func(context.Context, decision.Decision) error

func (f decisionRecorderFunc) Record(ctx context.Context, d decision.Decision) error {
	return f(ctx, d)
}

type capSink struct {
	got []decision.Decision
	err error
}

func (s *capSink) Record(_ context.Context, d decision.Decision) error {
	s.got = append(s.got, d)
	return s.err
}

func auditedScopeAuth(t *testing.T, rec auth.DecisionRecorder) auth.ClientAuth {
	t.Helper()
	reg := scope.NewRegistry()
	reg.Register(methodWrite, "files:write")
	reg.Freeze()
	enf := scope.NewEnforcer(reg, scope.ScopeAuthorizer(
		func(p *testPrincipal) []string { return p.scopes },
		scope.Exact(),
	))
	return auth.ScopeClientAuth(enf, auth.WithScopeAudit(rec, func(*testPrincipal) string { return "u1" }))
}

func TestScopeAudit_DecisionRecorderPort(t *testing.T) {
	t.Parallel()

	var got []decision.Decision
	recorderErr := errors.New("recorder failed")
	recorder := decisionRecorderFunc(func(_ context.Context, d decision.Decision) error {
		got = append(got, d)
		return recorderErr
	})
	ca := auditedScopeAuth(t, recorder)
	_, err := ca.ClientAuth(t.Context(), credsFor(methodWrite, &testPrincipal{scopes: []string{"files:write"}}))
	require.Error(t, err)
	require.Equal(t, codes.Internal, status.Code(err))
	require.Len(t, got, 1)
	require.True(t, got[0].Allowed)

	// A nil recorder leaves allowed calls unchanged.
	ca = auditedScopeAuth(t, nil)
	_, err = ca.ClientAuth(t.Context(), credsFor(methodWrite, &testPrincipal{scopes: []string{"files:write"}}))
	require.NoError(t, err)
}

func TestScopeAuditRecordsDecisions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		data        any
		method      string
		wantAllowed bool
		wantReason  string
		wantSubject string
	}{
		{"allow", &testPrincipal{scopes: []string{"files:write"}}, methodWrite, true, "", "u1"},
		{"deny missing scope", &testPrincipal{scopes: []string{"files:read"}}, methodWrite, false, "scope_denied", "u1"},
		{"deny unregistered", &testPrincipal{scopes: []string{"files:write"}}, "/files.v1.Files/Unknown", false, "scope_denied", "u1"},
		{"principal type mismatch", "not-a-principal", methodWrite, false, "principal_type_mismatch", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sink := &capSink{}
			rec := decision.NewRecorder(sink, decision.WithPolicyMode(decision.PolicyAll))
			ca := auditedScopeAuth(t, rec)

			_, _ = ca.ClientAuth(t.Context(), credsFor(tc.method, tc.data))

			require.Len(t, sink.got, 1)
			d := sink.got[0]
			require.Equal(t, tc.wantAllowed, d.Allowed)
			require.Equal(t, tc.method, d.Action)
			require.Equal(t, tc.wantReason, d.Reason)
			require.Equal(t, tc.wantSubject, d.Subject)
			require.Equal(t, "grpc", d.Attributes["transport"])
			require.False(t, d.Time.IsZero())
		})
	}
}

func TestScopeAuditDenyOnlyDropsAllows(t *testing.T) {
	t.Parallel()

	sink := &capSink{}
	ca := auditedScopeAuth(t, decision.NewRecorder(sink)) // default PolicyDenyOnly
	_, err := ca.ClientAuth(t.Context(), credsFor(methodWrite, &testPrincipal{scopes: []string{"files:write"}}))
	require.NoError(t, err)
	require.Empty(t, sink.got) // allowed decision not recorded under deny-only
}

func TestScopeAuditRequiredFailureFailsAllowedCall(t *testing.T) {
	t.Parallel()

	sink := &capSink{err: errors.New("sink down")}
	rec := decision.NewRecorder(sink, decision.WithPolicyMode(decision.PolicyAll), decision.WithFailureMode(decision.FailureRequired))
	ca := auditedScopeAuth(t, rec)

	_, err := ca.ClientAuth(t.Context(), credsFor(methodWrite, &testPrincipal{scopes: []string{"files:write"}}))
	require.Equal(t, codes.Internal, status.Code(err))
}
