// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package gcp

import (
	"testing"
)

func FuzzValidateProjectId(f *testing.F) {
	f.Add("")
	f.Add("a")
	f.Add("my-project")
	f.Add("project-123")
	f.Add("MY-PROJECT")
	f.Add("1project")
	f.Add("project-")

	f.Fuzz(func(t *testing.T, projectId string) {
		_ = validateProjectId(projectId)
	})
}

func FuzzValidateSecretKey(f *testing.F) {
	f.Add("")
	f.Add("ab")
	f.Add("my-secret")
	f.Add("_underscore")
	f.Add("1digit")
	f.Add("with.dot")

	f.Fuzz(func(t *testing.T, key string) {
		_ = validateSecretKey(key)
	})
}

func FuzzBuildGCPFilter(f *testing.F) {
	f.Add("env", "prod")
	f.Add("team", "backend")
	f.Add("", "")
	f.Add("key-with-special", "value=equals")

	f.Fuzz(func(t *testing.T, key, value string) {
		labels := map[string]string{key: value}
		_ = buildGCPFilter(labels)
	})
}
