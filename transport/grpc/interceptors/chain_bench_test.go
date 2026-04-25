// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package interceptors

import "testing"

func BenchmarkNewChain(b *testing.B) {
	for b.Loop() {
		NewChain(&NoOpInterceptor{}, &NoOpClientInterceptor{})
	}
}

func BenchmarkChain_ServerOptions(b *testing.B) {
	c := NewChain(&NoOpInterceptor{})
	for b.Loop() {
		c.ServerOptions() //nolint:errcheck
	}
}

func BenchmarkChain_ClientOptions(b *testing.B) {
	c := NewChain(&NoOpClientInterceptor{})
	for b.Loop() {
		c.ClientOptions() //nolint:errcheck
	}
}
