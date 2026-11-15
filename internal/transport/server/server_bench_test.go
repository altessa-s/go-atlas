// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package server

import "testing"

func BenchmarkNewBaseServer(b *testing.B) {
	for b.Loop() {
		NewBaseServer(WithAddress(":0"))
	}
}

func BenchmarkBaseServer_Protocol(b *testing.B) {
	s := NewBaseServer(WithAddress(":0"))
	var p string
	for b.Loop() {
		p = s.Protocol("http")
	}
	_ = p
}

func BenchmarkBaseServer_HasTLS(b *testing.B) {
	s := NewBaseServer(WithAddress(":0"))
	for b.Loop() {
		s.HasTLS()
	}
}
