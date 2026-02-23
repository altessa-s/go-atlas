// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package approle_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/security/vault/auth/approle"
)

func FuzzNew(f *testing.F) {
	f.Add("", "")
	f.Add("role-123", "secret-456")
	f.Add("  role  ", "  secret  ")
	f.Add("role-with-special-chars!@#", "secret")

	f.Fuzz(func(t *testing.T, roleID, secretID string) {
		m := approle.New(roleID, secretID)
		if m == nil {
			t.Fatal("New() returned nil")
		}
		if m.Name() != "approle" {
			t.Errorf("Name() = %q, want %q", m.Name(), "approle")
		}
		_ = m.Shutdown()
	})
}
