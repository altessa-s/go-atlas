// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/auth/scope/factory"
	"github.com/altessa-s/go-atlas/config"
)

func BenchmarkBuild(b *testing.B) {
	cfg := &config.ScopeRegistry{Rules: []config.ScopeRule{
		{Scope: "files:read", Keys: []string{"/files.v1.Files/Read", "/files.v1.Files/List"}},
		{Scope: "files:write", Keys: []string{"/files.v1.Files/Write", "/files.v1.Files/Delete"}},
		{Scope: "", Keys: []string{"/health.v1.Health/Check"}},
	}}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := factory.New(cfg).Build(); err != nil {
			b.Fatal(err)
		}
	}
}
