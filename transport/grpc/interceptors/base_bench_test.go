// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import "testing"

func BenchmarkNewBaseInterceptor(b *testing.B) {
	for b.Loop() {
		NewBaseInterceptor("bench", nil)
	}
}

func BenchmarkBaseInterceptor_ShouldIgnore(b *testing.B) {
	bi := NewBaseInterceptorWithFilter("bench", []string{"/grpc.health.v1.health/check"}, nil, nil)
	for b.Loop() {
		bi.ShouldIgnore("/grpc.health.v1.Health/Check")
	}
}

func BenchmarkBaseInterceptor_InternMethod(b *testing.B) {
	bi := NewBaseInterceptor("bench", nil)
	for b.Loop() {
		bi.InternMethod("/test/Method")
	}
}
