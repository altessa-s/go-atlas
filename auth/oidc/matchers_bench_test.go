// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import "testing"

func BenchmarkHasScope(b *testing.B) {
	claims := map[string]any{"scope": "read write admin openid profile email"}
	m := HasScope("admin")
	for b.Loop() {
		m(claims)
	}
}

func BenchmarkHasAnyScope(b *testing.B) {
	claims := map[string]any{"scope": "read write admin openid profile email"}
	m := HasAnyScope("delete", "manage", "admin", "superadmin")
	for b.Loop() {
		m(claims)
	}
}
