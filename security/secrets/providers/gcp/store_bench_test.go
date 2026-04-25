// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package gcp

import (
	"strings"
	"testing"
)

func BenchmarkValidateProjectId(b *testing.B) {
	id := "my-project-123"
	for b.Loop() {
		_ = validateProjectId(id)
	}
}

func BenchmarkValidateProjectId_Invalid(b *testing.B) {
	id := strings.Repeat("a", 31)
	for b.Loop() {
		_ = validateProjectId(id)
	}
}

func BenchmarkValidateServiceAccountPath(b *testing.B) {
	path := "/path/to/sa.json"
	for b.Loop() {
		_ = validateServiceAccountPath(path)
	}
}

func BenchmarkValidateSecretKey(b *testing.B) {
	key := "my-secret-key"
	for b.Loop() {
		_ = validateSecretKey(key)
	}
}

func BenchmarkFormatProjectPath(b *testing.B) {
	for b.Loop() {
		_ = formatProjectPath("my-project-123")
	}
}

func BenchmarkFormatSecretPath(b *testing.B) {
	for b.Loop() {
		_ = formatSecretPath("my-project", "my-secret")
	}
}

func BenchmarkFormatSecretVersionPath(b *testing.B) {
	for b.Loop() {
		_ = formatSecretVersionPath("my-project", "my-secret")
	}
}

func BenchmarkBuildGCPFilter(b *testing.B) {
	labels := map[string]string{"env": "prod", "team": "backend"}
	for b.Loop() {
		_ = buildGCPFilter(labels)
	}
}
