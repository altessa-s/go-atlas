// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/audit"
	"github.com/altessa-s/go-atlas/core/types/redacted"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/oidc"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authoidc "github.com/altessa-s/go-atlas/auth/oidc"
)

type fakeValidator struct {
	claims *oidc.Claims
	err    error
}

func (f fakeValidator) ValidateToken(_ context.Context, _ string) (*oidc.Claims, error) {
	return f.claims, f.err
}

type capSink struct {
	got []audit.Decision
	err error
}

func (s *capSink) Record(_ context.Context, d audit.Decision) error {
	s.got = append(s.got, d)
	return s.err
}

func tokenReq(token string) auth.Request {
	return auth.Request{
		Base:    auth.Base{AuthMethod: auth.MethodToken},
		Payload: &auth.TokenCredentials{Token: redacted.RedactedString(token)},
	}
}

func subjectOfClaims(c *oidc.Claims) string { return c.Subject }

func TestAuditRecordsDecisions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		validator   fakeValidator
		request     auth.Request
		wantAllowed bool
		wantReason  string
		wantSubject string
	}{
		{
			name:        "success",
			validator:   fakeValidator{claims: &oidc.Claims{Subject: "user1"}},
			request:     tokenReq("valid-token"),
			wantAllowed: true,
			wantReason:  "",
			wantSubject: "user1",
		},
		{
			name:        "missing token",
			validator:   fakeValidator{},
			request:     auth.Request{Base: auth.Base{AuthMethod: "other"}},
			wantAllowed: false,
			wantReason:  "missing_token",
			wantSubject: "",
		},
		{
			name:        "invalid token",
			validator:   fakeValidator{err: authoidc.ErrTokenInvalid},
			request:     tokenReq("bad"),
			wantAllowed: false,
			wantReason:  "invalid_token",
			wantSubject: "",
		},
		{
			name:        "revoked token",
			validator:   fakeValidator{err: authoidc.ErrTokenRevoked},
			request:     tokenReq("revoked"),
			wantAllowed: false,
			wantReason:  "revoked",
			wantSubject: "",
		},
		{
			name:        "unmapped error",
			validator:   fakeValidator{err: errors.New("boom")},
			request:     tokenReq("bad"),
			wantAllowed: false,
			wantReason:  "error",
			wantSubject: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sink := &capSink{}
			rec := audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll))
			fn := oidc.AuthFunc(tc.validator, oidc.WithAudit(rec, subjectOfClaims))

			_, _ = fn(t.Context(), tc.request)

			require.Len(t, sink.got, 1)
			d := sink.got[0]
			require.Equal(t, tc.wantAllowed, d.Allowed)
			require.Equal(t, "validate_token", d.Action)
			require.Equal(t, tc.wantReason, d.Reason)
			require.Equal(t, tc.wantSubject, d.Subject)
			require.Equal(t, "grpc", d.Attributes["transport"])
			require.False(t, d.Time.IsZero())
		})
	}
}

func TestAuditNilSubjectOf(t *testing.T) {
	t.Parallel()

	sink := &capSink{}
	rec := audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll))
	fn := oidc.AuthFunc(fakeValidator{claims: &oidc.Claims{Subject: "user1"}}, oidc.WithAudit(rec, nil))

	_, err := fn(t.Context(), tokenReq("valid-token"))
	require.NoError(t, err)
	require.Len(t, sink.got, 1)
	require.Empty(t, sink.got[0].Subject)
}

func TestAuditDenyOnlyDropsAllows(t *testing.T) {
	t.Parallel()

	sink := &capSink{}
	rec := audit.NewRecorder(sink) // default PolicyDenyOnly
	fn := oidc.AuthFunc(fakeValidator{claims: &oidc.Claims{Subject: "user1"}}, oidc.WithAudit(rec, subjectOfClaims))

	_, err := fn(t.Context(), tokenReq("valid-token"))
	require.NoError(t, err)
	require.Empty(t, sink.got) // allowed decision not recorded under deny-only
}

func TestAuditRequiredFailureFailsAllowedCall(t *testing.T) {
	t.Parallel()

	sink := &capSink{err: errors.New("sink down")}
	rec := audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll), audit.WithFailureMode(audit.FailureRequired))
	fn := oidc.AuthFunc(fakeValidator{claims: &oidc.Claims{Subject: "user1"}}, oidc.WithAudit(rec, subjectOfClaims))

	_, err := fn(t.Context(), tokenReq("valid-token"))
	require.Equal(t, codes.Internal, status.Code(err))
}
