// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package approle_test

import (
	"testing"

	"github.com/stretchr/testify/require"

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
			require.NotNil(t, m)
			require.Equal(t, "approle", m.Name())
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
			require.NotNil(t, m)
			require.Equal(t, tt.mountPath, m.MountPath())
		})
	}
}

func TestNew_WithMountPathPointer(t *testing.T) {
	path := "/auth/custom-approle"
	m := approle.New("role-123", "secret-456", approle.WithMountPath(&path))
	require.NotNil(t, m)
	require.Equal(t, path, m.MountPath())
}

func TestAuthMethod_Shutdown(t *testing.T) {
	m := approle.New("role", "secret")
	require.NoError(t, m.Shutdown())
}
