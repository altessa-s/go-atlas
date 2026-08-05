// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package naming derives exported option names from private struct field
// names. It is called by the optgen parser when a field carries no explicit
// `opt` tag.
//
// # Initialisms
//
// Go style requires an initialism to keep a consistent case throughout an
// identifier: TTL, not Ttl; HTTPClient, not HttpClient. A naive
// capitalize-the-first-letter rule violates that for every field whose name
// starts with or contains one, which is how the repo ended up with a public
// WithTtl. [OptionName] upper-cases every word that is a known initialism.
//
// # Overriding
//
// The table is deliberate, not exhaustive. A field whose spelling the table
// gets wrong overrides it with the `opt` tag, which the parser prefers over
// this package:
//
//	type options struct {
//	    ttl  time.Duration                    // -> WithTTL
//	    uid  string        `opt:"Uid"`        // -> WithUid
//	}
//
// # Usage
//
//	name := naming.OptionName("httpClient") // "HTTPClient"
package naming
