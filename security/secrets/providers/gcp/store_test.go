// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package gcp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateProjectId(t *testing.T) {
	tests := []struct {
		name      string
		projectId string
		wantErr   error
	}{
		{"valid simple", "my-project", nil},
		{"valid single char", "a", nil},
		{"valid with digits", "project-123", nil},
		{"empty", "", ErrInvalidProjectId},
		{"too long", strings.Repeat("a", 31), ErrInvalidProjectId},
		{"max length valid", strings.Repeat("a", 30), nil},
		{"starts with digit", "1project", ErrInvalidProjectId},
		{"ends with hyphen", "project-", ErrInvalidProjectId},
		{"uppercase", "MyProject", ErrInvalidProjectId},
		{"contains underscore", "my_project", ErrInvalidProjectId},
		{"contains space", "my project", ErrInvalidProjectId},
		{"only hyphen", "-", ErrInvalidProjectId},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProjectId(tt.projectId)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateServiceAccountPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr error
	}{
		{"valid path", "/path/to/sa.json", nil},
		{"empty", "", ErrInvalidServiceAccount},
		{"whitespace only", "   ", ErrInvalidServiceAccount},
		{"tab only", "\t", ErrInvalidServiceAccount},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateServiceAccountPath(tt.path)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateSecretKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid key", "my-secret", false},
		{"valid with underscore", "_my_secret", false},
		{"valid letters only", "MySecret", false},
		{"empty", "", true},
		{"single char", "a", true},
		{"starts with digit", "1secret", true},
		{"contains dot", "my.secret", true},
		{"contains space", "my secret", true},
		{"too long", strings.Repeat("a", 256), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSecretKey(tt.key)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestFormatProjectPath(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		want      string
	}{
		{"basic", "my-project", "projects/my-project"},
		{"with digits", "project-123", "projects/project-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, formatProjectPath(tt.projectID))
		})
	}
}

func TestFormatSecretPath(t *testing.T) {
	tests := []struct {
		name       string
		projectID  string
		secretName string
		want       string
	}{
		{"basic", "proj", "secret", "projects/proj/secrets/secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, formatSecretPath(tt.projectID, tt.secretName))
		})
	}
}

func TestFormatSecretVersionPath(t *testing.T) {
	got := formatSecretVersionPath("proj", "sec")
	require.Equal(t, "projects/proj/secrets/sec/versions/latest", got)
}

func TestBuildGCPFilter(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{"empty", nil, ""},
		{"empty map", map[string]string{}, ""},
		{"single label", map[string]string{"env": "prod"}, "(labels.env=prod)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGCPFilter(tt.labels)
			if tt.want == "" {
				require.Empty(t, got)
				return
			}
			// For single label, exact match
			if len(tt.labels) == 1 {
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestStorageName(t *testing.T) {
	// Can't construct Storage without a client, but Name() doesn't use client
	s := &Storage[string]{}
	require.Equal(t, "gcp", s.Name())
}
