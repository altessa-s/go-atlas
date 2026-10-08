// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package fieldpolicy holds the field-name policy shared by the request DSL
// packages under data/ (orderby, projection): a frozen allow-list with
// ".*" wildcard prefixes, and an exact + prefix mapping from DSL names to
// storage names.
//
// # Allow-list
//
//	allow := fieldpolicy.NewAllowList("createdAt", "address.*")
//	allow.Allows("address.city") // true
//	allow.Allows("address")      // false: ".*" matches subpaths only
//
// # Mapping
//
//	m := fieldpolicy.Mapping{}.
//	    WithExact(map[string]string{"createdAt": "created_at"}).
//	    WithPrefixes(map[string]string{"address.": "addr."})
//	m.Apply("address.city") // "addr.city"
//
// Both types are immutable values and safe for concurrent use.
package fieldpolicy
