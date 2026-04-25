// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package lockbox

import (
	"testing"
)

func FuzzValidateLockboxFolderId(f *testing.F) {
	f.Add("")
	f.Add("b1g2h3j4k5l6m7n8o9p0")
	f.Add("folder-123-abc")
	f.Add("folder_123")

	f.Fuzz(func(t *testing.T, folderId string) {
		_ = validateLockboxFolderId(folderId)
	})
}

func FuzzValidateLockboxKeyId(f *testing.F) {
	f.Add("")
	f.Add("aje1234567890abcdef")
	f.Add("key-123")
	f.Add("key!@#")

	f.Fuzz(func(t *testing.T, keyId string) {
		_ = validateLockboxKeyId(keyId)
	})
}

func FuzzValidateLockboxServiceAccountId(f *testing.F) {
	f.Add("")
	f.Add("aje1234567890abcdef")
	f.Add("sa-123-abc")

	f.Fuzz(func(t *testing.T, saId string) {
		_ = validateLockboxServiceAccountId(saId)
	})
}

func FuzzValidateLockboxSecretKey(f *testing.F) {
	f.Add("")
	f.Add("ab")
	f.Add("my-secret")
	f.Add("_underscore")
	f.Add("1digit")

	f.Fuzz(func(t *testing.T, key string) {
		_ = validateLockboxSecretKey(key)
	})
}
