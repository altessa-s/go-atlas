// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package token_test

import (
	"testing"

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
			if m == nil {
				t.Fatal("New() returned nil")
			}
			if got := m.Name(); got != tt.wantName {
				t.Errorf("Name() = %q, want %q", got, tt.wantName)
			}
		})
	}
}

func TestAuthMethod_Shutdown(t *testing.T) {
	m := token.New("hvs.abc123")
	if err := m.Shutdown(); err != nil {
		t.Errorf("Shutdown() = %v, want nil", err)
	}
}

func TestAuthMethod_Name(t *testing.T) {
	m := token.New("test")
	if got := m.Name(); got != "token" {
		t.Errorf("Name() = %q, want %q", got, "token")
	}
}
