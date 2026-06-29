// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/audit"
)

type capScopeSink struct {
	got []audit.Decision
	err error
}

func (s *capScopeSink) Record(_ context.Context, d audit.Decision) error {
	s.got = append(s.got, d)
	return s.err
}

func auditedScope(rec *audit.Recorder) http.Handler {
	return ScopeMiddleware(newScopeEnforcer(), scopeKey,
		WithScopeAudit(rec, func(*scopePrincipal) string { return "u1" }))(okHandler())
}

func TestScopeMiddlewareAuditRecords(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		data        any
		path        string
		wantCode    int
		wantAllowed bool
		wantReason  string
		wantSubject string
	}{
		{"allow", &scopePrincipal{scopes: []string{"files:read"}}, "/files", http.StatusOK, true, "", "u1"},
		{"deny missing scope", &scopePrincipal{scopes: []string{"files:write"}}, "/files", http.StatusForbidden, false, "scope_denied", "u1"},
		{"deny unregistered", &scopePrincipal{scopes: []string{"files:read"}}, "/secret", http.StatusForbidden, false, "scope_denied", "u1"},
		{"no principal", nil, "/files", http.StatusForbidden, false, "principal_type_mismatch", ""},
		{"wrong type", "not-a-principal", "/files", http.StatusForbidden, false, "principal_type_mismatch", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sink := &capScopeSink{}
			rec := audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll))
			code := serveScope(auditedScope(rec), http.MethodGet, tc.path, tc.data)
			require.Equal(t, tc.wantCode, code)
			require.Len(t, sink.got, 1)
			d := sink.got[0]
			require.Equal(t, tc.wantAllowed, d.Allowed)
			require.Equal(t, "GET "+tc.path, d.Action)
			require.Equal(t, tc.wantReason, d.Reason)
			require.Equal(t, tc.wantSubject, d.Subject)
			require.Equal(t, "http", d.Attributes["transport"])
		})
	}
}

func TestScopeMiddlewareAuditDenyOnlyDropsAllow(t *testing.T) {
	t.Parallel()
	sink := &capScopeSink{}
	code := serveScope(auditedScope(audit.NewRecorder(sink)), // default deny-only
		http.MethodGet, "/files", &scopePrincipal{scopes: []string{"files:read"}})
	require.Equal(t, http.StatusOK, code)
	require.Empty(t, sink.got)
}

func TestScopeMiddlewareAuditRequiredFailureFails(t *testing.T) {
	t.Parallel()
	sink := &capScopeSink{err: errors.New("sink down")}
	rec := audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll), audit.WithFailureMode(audit.FailureRequired))
	code := serveScope(auditedScope(rec), http.MethodGet, "/files", &scopePrincipal{scopes: []string{"files:read"}})
	require.Equal(t, http.StatusInternalServerError, code)
}
