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
	"github.com/altessa-s/go-atlas/transport/http/server/middlewares/auth/static"
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
	want := userInfo{ID: "user1", Role: "admin", Email: "user1@example.com"}
	store := static.NewInMemoryStore(static.WithInitialTokens(map[string]any{
		"valid_token": want,
	}))
	fn := static.AuthFunc(store, static.WithAudit(
		audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll)),
		subjectFromUserInfo,
	))

	data, err := fn.Authenticate(t.Context(), "valid_token")
	require.NoError(t, err)
	require.Equal(t, want, data)

	decisions := sink.all()
	require.Len(t, decisions, 1)
	d := decisions[0]
	require.True(t, d.Allowed)
	require.Equal(t, "authenticate", d.Action)
	require.Equal(t, "user1", d.Subject)
	require.Empty(t, d.Reason)
	require.Equal(t, "http", d.Attributes["transport"])
	require.False(t, d.Time.IsZero())
}

func TestAuthFunc_Audit_InvalidTokenDenial(t *testing.T) {
	t.Parallel()
	sink := &captureSink{}
	store := static.NewInMemoryStore()
	fn := static.AuthFunc(store, static.WithAudit(
		audit.NewRecorder(sink, audit.WithPolicyMode(audit.PolicyAll)),
		subjectFromUserInfo,
	))

	data, err := fn.Authenticate(t.Context(), "bad")
	require.Error(t, err)
	require.Nil(t, data)

	decisions := sink.all()
	require.Len(t, decisions, 1)
	d := decisions[0]
	require.False(t, d.Allowed)
	require.Equal(t, "invalid_token", d.Reason)
	require.Empty(t, d.Subject)
	require.Equal(t, "http", d.Attributes["transport"])
	require.False(t, d.Time.IsZero())
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

	data, err := fn.Authenticate(t.Context(), "valid_token")
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

	data, err := fn.Authenticate(t.Context(), "valid_token")
	require.Error(t, err)
	require.Nil(t, data)
	require.ErrorIs(t, err, audit.ErrAuditFailed)
}
