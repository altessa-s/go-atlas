// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import "testing"

func BenchmarkBase_IsAuthenticated(b *testing.B) {
	base := &Base{AuthMethod: MethodToken}
	for b.Loop() {
		base.IsAuthenticated()
	}
}

func BenchmarkRequest_TokenCredentials(b *testing.B) {
	req := Request{Base: Base{AuthMethod: MethodToken}, Payload: &TokenCredentials{Token: "abc"}}
	for b.Loop() {
		req.TokenCredentials()
	}
}
