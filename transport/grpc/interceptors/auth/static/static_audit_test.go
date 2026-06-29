// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package static_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/audit"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth"
	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/auth/static"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// captureSink records every decision it receives for later assertions.
type captureSink struct {
	mu        sync.Mutex
	decisions []audit.Decision
}

func (s *captureSink) Record(_ context.Context, d audit.Decision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decisions = append(s.decisions, d)
	return nil
}

func (s *captureSink) all() []audit.Decision {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]audit.Decision(nil), s.decisions...)
}

// failingSink always fails, to exercise the FailureRequired path.
type failingSink struct{}

func (failingSink) Record(context.Context, audit.Decision) error {
	return errors.New("sink unavailable")
}

func subjectFromUserInfo(v any) string {
	u, _ := v.(userInfo)
	return u.ID
}

func TestAuthFunc_Audit_Success(t *testing.T) {
	t.Parallel()
	sink := &captureSink{}
	want := userInfo{ID: "user1", Role: "admin"}
	store := static.NewInMemoryStore(static.WithInitialTokens(map[string]any{
		"valid_token": want,
	}))
	fn := static.AuthFunc(store, static.WithAudit(
		audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll)),
		subjectFromUserInfo,
	))

	data, err := fn(t.Context(), auth.Request{
		Base:    auth.Base{AuthMethod: auth.MethodToken},
		Payload: &auth.TokenCredentials{Token: "valid_token"},
	})
	require.NoError(t, err)
	require.Equal(t, want, data)

	decisions := sink.all()
	require.Len(t, decisions, 1)
	d := decisions[0]
	require.True(t, d.Allowed)
	require.Equal(t, "authenticate", d.Action)
	require.Equal(t, "user1", d.Subject)
	require.Empty(t, d.Reason)
	require.Equal(t, "grpc", d.Attributes["transport"])
	require.False(t, d.Time.IsZero())
}

func TestAuthFunc_Audit_Denials(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		req        auth.Request
		wantReason string
	}{
		{
			name:       "invalid token",
			req:        auth.Request{Base: auth.Base{AuthMethod: auth.MethodToken}, Payload: &auth.TokenCredentials{Token: "bad"}},
			wantReason: "invalid_token",
		},
		{
			name:       "missing credentials",
			req:        auth.Request{Base: auth.Base{AuthMethod: "other"}},
			wantReason: "missing_token",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sink := &captureSink{}
			store := static.NewInMemoryStore()
			fn := static.AuthFunc(store, static.WithAudit(
				audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll)),
				subjectFromUserInfo,
			))

			data, err := fn(t.Context(), tc.req)
			require.Error(t, err)
			require.Nil(t, data)

			decisions := sink.all()
			require.Len(t, decisions, 1)
			d := decisions[0]
			require.False(t, d.Allowed)
			require.Equal(t, tc.wantReason, d.Reason)
			require.Empty(t, d.Subject)
			require.Equal(t, "grpc", d.Attributes["transport"])
			require.False(t, d.Time.IsZero())
		})
	}
}

func TestAuthFunc_Audit_DenyOnlyDropsSuccess(t *testing.T) {
	t.Parallel()
	sink := &captureSink{}
	want := userInfo{ID: "user1", Role: "admin"}
	store := static.NewInMemoryStore(static.WithInitialTokens(map[string]any{
		"valid_token": want,
	}))
	// Default policy is deny-only: a granted decision must be dropped.
	fn := static.AuthFunc(store, static.WithAudit(audit.NewRecorder(sink), subjectFromUserInfo))

	data, err := fn(t.Context(), auth.Request{
		Base:    auth.Base{AuthMethod: auth.MethodToken},
		Payload: &auth.TokenCredentials{Token: "valid_token"},
	})
	require.NoError(t, err)
	require.Equal(t, want, data)
	require.Empty(t, sink.all())
}

func TestAuthFunc_Audit_FailureRequiredOnSuccess(t *testing.T) {
	t.Parallel()
	want := userInfo{ID: "user1", Role: "admin"}
	store := static.NewInMemoryStore(static.WithInitialTokens(map[string]any{
		"valid_token": want,
	}))
	fn := static.AuthFunc(store, static.WithAudit(
		audit.NewRecorder(failingSink{},
			audit.WithPolicyMode(audit.PolicyAll),
			audit.WithFailureMode(audit.FailureRequired),
		),
		subjectFromUserInfo,
	))

	data, err := fn(t.Context(), auth.Request{
		Base:    auth.Base{AuthMethod: auth.MethodToken},
		Payload: &auth.TokenCredentials{Token: "valid_token"},
	})
	require.Error(t, err)
	require.Nil(t, data)
	require.Equal(t, codes.Internal, status.Code(err))
}
