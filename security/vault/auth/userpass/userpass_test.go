// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package userpass_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/security/vault/auth/userpass"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
	}{
		{"basic", "admin", "password123"},
		{"with spaces", "  admin  ", "  pass  "},
		{"empty values", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := userpass.New(tt.username, tt.password)
			if m == nil {
				t.Fatal("New() returned nil")
			}
			if got := m.Name(); got != "userpass" {
				t.Errorf("Name() = %q, want %q", got, "userpass")
			}
		})
	}
}

func TestNew_WithMountPath(t *testing.T) {
	tests := []struct {
		name      string
		mountPath string
	}{
		{"string mount path", "/auth/userpass"},
		{"custom path", "/custom/userpass"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := userpass.New("admin", "pass", userpass.WithMountPath(tt.mountPath))
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
	path := "/auth/custom-userpass"
	m := userpass.New("admin", "pass", userpass.WithMountPath(&path))
	if m == nil {
		t.Fatal("New() returned nil")
	}
	if got := m.MountPath(); got != path {
		t.Errorf("MountPath() = %q, want %q", got, path)
	}
}

func TestAuthMethod_Shutdown(t *testing.T) {
	m := userpass.New("admin", "pass")
	if err := m.Shutdown(); err != nil {
		t.Errorf("Shutdown() = %v, want nil", err)
	}
}
