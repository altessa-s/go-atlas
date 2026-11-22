// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package document applies MongoDB client-side field level encryption to
// fields selected by a store encryption model or a struct tag.
//
// # Usage
//
//	fields, err := document.New(tagName, models, encrypt)
//
// The store binds fields to the BSON builder. The builder resolves field
// names and encodes the selected plaintext with the effective registry.
// The store encryptor receives the raw BSON value and owns key management.
package document
