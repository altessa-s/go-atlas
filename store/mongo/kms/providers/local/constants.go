// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package kmslocal

// ProviderName is the identifier for local KMS provider
const ProviderName = "local"

// MasterKey is the credential field name for local master key
const MasterKey = "key"

// RequiredMasterKeyLength is the required length in bytes for local KMS master keys.
// MongoDB Client-Side Field Level Encryption (CSFLE) requires exactly 96 bytes for AES-256.
const RequiredMasterKeyLength = 96
