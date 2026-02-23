// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package userpass_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/security/vault/auth/userpass"
)

func FuzzNew(f *testing.F) {
	f.Add("", "")
	f.Add("admin", "password123")
	f.Add("  user  ", "  pass  ")
	f.Add("user@domain.com", "p@$$w0rd!")

	f.Fuzz(func(t *testing.T, username, password string) {
		m := userpass.New(username, password)
		if m == nil {
			t.Fatal("New() returned nil")
		}
		if m.Name() != "userpass" {
			t.Errorf("Name() = %q, want %q", m.Name(), "userpass")
		}
		_ = m.Shutdown()
	})
}
