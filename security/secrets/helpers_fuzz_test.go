// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package secrets_test

import (
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/security/secrets"
)

// FuzzValidateSecretKeyMatchesTheDocumentedShape restates the accepted key
// format independently and requires the validator to agree.
//
// The key names a secret in an external backend and is often assembled from
// configuration or a tenant identifier. A validator looser than its documented
// shape lets a caller address a secret it was not meant to; one stricter
// rejects legitimate names at deploy time, far from the config that chose them.
func FuzzValidateSecretKeyMatchesTheDocumentedShape(f *testing.F) {
	f.Add("database.password")
	f.Add("")
	f.Add("a")
	f.Add("with space")
	f.Add(".")
	f.Add("..")
	f.Add("..0")
	f.Add("UPPER_case-123")
	f.Add("ключ")
	f.Add("a\x00b")
	f.Add(strings.Repeat("a", 4096))

	f.Fuzz(func(t *testing.T, key string) {
		want := secrets.ValidateKeyLength(key) && isAllowedKeyText(key) && key != "." && key != ".."

		require.Equal(t, want, secrets.ValidateSecretKey(key) == nil,
			"the validator disagrees with the documented key shape: %q", key)
	})
}

// isAllowedKeyText is an independent statement of the documented character set:
// alphanumerics, underscore, hyphen and dot.
func isAllowedKeyText(key string) bool {
	if key == "" {
		return false
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '.':
		default:
			return false
		}
	}
	return true
}

// FuzzAcceptedKeysCannotEscapeTheirPath is the traversal oracle, stated the way
// the hazard actually arises.
//
// The Vault provider addresses a secret as path.Join(secretPath, key), so the
// question is not whether the key contains ".." — "..0" does and is harmless —
// but whether joining it lands outside the configured path. Asking path.Join
// directly is exact and needs no grammar of its own.
func FuzzAcceptedKeysCannotEscapeTheirPath(f *testing.F) {
	f.Add("normal")
	f.Add("..")
	f.Add(".")
	f.Add("..0")
	f.Add("a.b.c")
	f.Add("...")

	const base = "secret/data/app"

	f.Fuzz(func(t *testing.T, key string) {
		if secrets.ValidateSecretKey(key) != nil {
			return
		}

		joined := path.Join(base, key)
		require.True(t, strings.HasPrefix(joined, base+"/"),
			"an accepted key escapes its configured path: %q joined to %q", key, joined)
	})
}
