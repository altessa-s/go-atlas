// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package approle_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/security/vault/auth/approle"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		roleID   string
		secretID string
	}{
		{"basic", "role-123", "secret-456"},
		{"with spaces", "  role-123  ", "  secret-456  "},
		{"empty values", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := approle.New(tt.roleID, tt.secretID)
			if m == nil {
				t.Fatal("New() returned nil")
			}
			if got := m.Name(); got != "approle" {
				t.Errorf("Name() = %q, want %q", got, "approle")
			}
		})
	}
}

func TestNew_WithMountPath(t *testing.T) {
	tests := []struct {
		name      string
		mountPath string
	}{
		{"string mount path", "/auth/approle"},
		{"custom path", "/custom/approle"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := approle.New("role-123", "secret-456", approle.WithMountPath(tt.mountPath))
			if m == nil {
				t.Fatal("New() returned nil")
			}
			if got := m.MountPath(); got != tt.mountPath {
				t.Errorf("MountPath() = %q, want %q", got, tt.mountPath)
			}
		})
	}
}

func TestNew_WithMountPathPointer(t *testing.T) {
	path := "/auth/custom-approle"
	m := approle.New("role-123", "secret-456", approle.WithMountPath(&path))
	if m == nil {
		t.Fatal("New() returned nil")
	}
	if got := m.MountPath(); got != path {
		t.Errorf("MountPath() = %q, want %q", got, path)
	}
}

func TestAuthMethod_Shutdown(t *testing.T) {
	m := approle.New("role", "secret")
	if err := m.Shutdown(); err != nil {
		t.Errorf("Shutdown() = %v, want nil", err)
	}
}
