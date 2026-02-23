// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package token_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/security/vault/auth/token"
)

func FuzzNew(f *testing.F) {
	f.Add("")
	f.Add("hvs.abc123")
	f.Add("  hvs.abc123  ")
	f.Add("root")
	f.Add("s.1234567890abcdef")

	f.Fuzz(func(t *testing.T, tok string) {
		m := token.New(tok)
		if m == nil {
			t.Fatal("New() returned nil for input")
		}
		if m.Name() != "token" {
			t.Errorf("Name() = %q, want %q", m.Name(), "token")
		}
		_ = m.Shutdown()
	})
}
