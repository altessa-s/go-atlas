// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package pagingconfig defines the schema of signed page tokens shared by every
// paginated component (audit queries, scheduler lists, service List RPCs): the
// signing key, the retired keys still accepted after a rotation, the key
// encoding, the token lifetime and token encryption.
//
// Schemas carry yaml and default tags read by
// github.com/altessa-s/go-nomos/loader and implement Validate;
// github.com/altessa-s/go-atlas/factory/ext/aip/pagination/pagetoken builds a page-token codec from
// [Config].
//
// # Key encoding
//
// keyEncoding selects how a configured key becomes key bytes: raw (default)
// uses the string bytes, base64 decodes standard padded base64, which lets a
// key carry arbitrary bytes. Either way a key must be at least [MinKeyLength]
// bytes after decoding.
//
// # Encryption
//
// encryption.enabled issues encrypted tokens whose payload and bindings the
// client cannot read; signed tokens are still accepted, so it is enabled
// replica by replica. encryption.strict, which requires enabled, also rejects
// signed tokens: set it once every signed token in circulation has expired.
// The encryption keys are derived from the signing keys and rotate with them.
//
// # Own field names
//
// A schema that keeps signing keys under its own field names (for example
// next to other secrets) validates them with [KeyRule] and decodes them with
// [DecodeKey], then hands a [Config] to the factory.
//
// Key types: [Config], [Encryption], [KeyEncoding].
package pagingconfig
