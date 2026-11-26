// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth

import "testing"

func BenchmarkPerRPCCredentials_GetRequestMetadata(b *testing.B) {
	creds, _ := NewPerRPCCredentials(StaticTokenProvider("tok"))
	for b.Loop() {
		creds.GetRequestMetadata(b.Context()) //nolint:errcheck
	}
}
