// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"strings"
	"testing"

	"github.com/altessa-s/go-atlas/security/secrets"
)

func FuzzValidateSecretKey(f *testing.F) {
	f.Add("")
	f.Add("ab")
	f.Add("valid.key")
	f.Add("!@#")
	f.Add(strings.Repeat("a", 256))

	f.Fuzz(func(t *testing.T, key string) {
		_ = secrets.ValidateSecretKey(key)
	})
}
