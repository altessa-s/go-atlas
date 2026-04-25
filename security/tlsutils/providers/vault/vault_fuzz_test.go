// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package tlsvault_test

import (
	"testing"

	tlsvault "github.com/altessa-s/go-atlas/security/tlsutils/providers/vault"
)

func FuzzWithEndpoint(f *testing.F) {
	f.Add("https://vault.example.com")
	f.Add("http://localhost:8200")
	f.Add("://invalid")
	f.Add("")
	f.Add("vault.example.com")

	f.Fuzz(func(t *testing.T, endpoint string) {
		// Should not panic regardless of input
		_, _ = tlsvault.New(
			tlsvault.WithEndpoint(endpoint),
			tlsvault.WithStaticToken("s.token"),
			tlsvault.WithRole("role"),
			tlsvault.WithCommonName("test"),
		)
	})
}
