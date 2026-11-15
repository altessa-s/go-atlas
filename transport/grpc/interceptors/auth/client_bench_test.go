// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import "testing"

func BenchmarkClientInterceptor_AttachToken(b *testing.B) {
	ci := ClientInterceptor(StaticTokenProvider("tok"))
	inner := ci.(*clientInterceptor)
	for b.Loop() {
		inner.attachToken(b.Context()) //nolint:errcheck
	}
}
