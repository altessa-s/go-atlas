// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package auth_test

import (
	"testing"

	"github.com/altessa-s/go-atlas/security/vault/auth"
)

func TestBaseMethod_Name(t *testing.T) {
	base := auth.NewBaseMethod("approle")
	if got := base.Name(); got != "approle" {
		t.Errorf("Name() = %q, want %q", got, "approle")
	}
}

func TestBaseMethod_MountPath(t *testing.T) {
	base := auth.NewBaseMethod("approle")
	base.SetMountPath("  /auth/approle  ")
	if got := base.MountPath(); got != "/auth/approle" {
		t.Errorf("MountPath() = %q, want %q", got, "/auth/approle")
	}
}

func TestBaseMethod_Shutdown(t *testing.T) {
	base := auth.NewBaseMethod("approle")
	if err := base.Shutdown(); err != nil {
		t.Errorf("Shutdown() = %v, want nil", err)
	}
}
