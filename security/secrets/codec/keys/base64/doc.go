// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package base64 provides a KeyDecoder implementation using base64 URL-safe encoding.
//
// Keys are encoded using base64.RawURLEncoding to ensure they are safe for use
// in URL paths of secret storage systems.
//
// # Usage
//
//	decoder := base64.NewKeyDecoder()
//	encoded, _ := decoder.Encode("my/secret/key")
//	original, err := decoder.Decode(encoded)
package base64
