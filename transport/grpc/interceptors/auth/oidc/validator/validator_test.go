// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package validator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockProvider struct {
	claims map[string]any
	err    error
}

func (m *mockProvider) ValidateToken(_ context.Context, _ string) (map[string]any, error) {
	return m.claims, m.err
}

func TestDefaultValidator_ValidateToken_Success(t *testing.T) {
	p := &mockProvider{claims: map[string]any{"sub": "user1", "iss": "https://issuer"}}
	v := NewDefaultValidator(p)

	claims, err := v.ValidateToken(t.Context(), "valid-token")
	require.NoError(t, err)
	require.Equal(t, "user1", claims.Subject)
	require.Equal(t, "https://issuer", claims.Issuer)
}

func TestDefaultValidator_ValidateToken_Error(t *testing.T) {
	p := &mockProvider{err: status.Error(codes.Unauthenticated, "bad")}
	v := NewDefaultValidator(p)

	_, err := v.ValidateToken(t.Context(), "bad-token")
	require.NotNil(t, err, "expected error")
	st, ok := status.FromError(err)
	require.True(t, ok, "expected Unauthenticated, got %v", err)
	require.Equal(t, codes.Unauthenticated, st.Code())
}
