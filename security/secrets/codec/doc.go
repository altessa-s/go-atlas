// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package codec provides interfaces for encoding and decoding secret keys and values.
// Enables pluggable serialization formats for secret storage systems.
//
// Example:
//
//	var keyDecoder codec.KeyDecoder = base64.NewDecoder()
//	encoded, _ := keyDecoder.Encode("my-secret-key")
//	decoded, _ := keyDecoder.Decode(encoded)
package codec
