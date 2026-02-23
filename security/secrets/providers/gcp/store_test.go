// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package gcp

import (
	"strings"
	"testing"
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
				if err != tt.wantErr {
					t.Errorf("validateProjectId(%q) = %v, want %v", tt.projectId, err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("validateProjectId(%q) unexpected error: %v", tt.projectId, err)
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
				if err != tt.wantErr {
					t.Errorf("validateServiceAccountPath(%q) = %v, want %v", tt.path, err, tt.wantErr)
				}
			} else if err != nil {
				t.Errorf("validateServiceAccountPath(%q) unexpected error: %v", tt.path, err)
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
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSecretKey(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
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
			if got := formatProjectPath(tt.projectID); got != tt.want {
				t.Errorf("formatProjectPath(%q) = %q, want %q", tt.projectID, got, tt.want)
			}
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
			if got := formatSecretPath(tt.projectID, tt.secretName); got != tt.want {
				t.Errorf("formatSecretPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatSecretVersionPath(t *testing.T) {
	got := formatSecretVersionPath("proj", "sec")
	want := "projects/proj/secrets/sec/versions/latest"
	if got != want {
		t.Errorf("formatSecretVersionPath() = %q, want %q", got, want)
	}
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
				if got != "" {
					t.Errorf("buildGCPFilter() = %q, want empty", got)
				}
				return
			}
			// For single label, exact match
			if len(tt.labels) == 1 && got != tt.want {
				t.Errorf("buildGCPFilter() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStorageName(t *testing.T) {
	// Can't construct Storage without a client, but Name() doesn't use client
	s := &Storage[string]{}
	if got := s.Name(); got != "gcp" {
		t.Errorf("Name() = %q, want %q", got, "gcp")
	}
}
