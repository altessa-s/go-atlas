// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package token_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/vault/auth/token"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		token    string
		wantName string
	}{
		{"basic token", "hvs.abc123", "token"},
		{"token with spaces", "  hvs.abc123  ", "token"},
		{"empty token", "", "token"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := token.New(tt.token)
			require.NotNil(t, m)
			require.Equal(t, tt.wantName, m.Name())
		})
	}
}

func TestAuthMethod_Shutdown(t *testing.T) {
	m := token.New("hvs.abc123")
	require.NoError(t, m.Shutdown())
}

func TestAuthMethod_Name(t *testing.T) {
	m := token.New("test")
	require.Equal(t, "token", m.Name())
}
