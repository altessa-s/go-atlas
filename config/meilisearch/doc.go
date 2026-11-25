// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package meilisearchconfig defines the Meilisearch connection schema.
//
// Schemas carry yaml and default tags read by
// github.com/altessa-s/go-nomos/loader, expose Default* constructors
// where defaults exist, and implement Validate; component factories map them
// to generated options.
//
// The base URL must use HTTP or HTTPS and may include a path prefix.
// TLS settings require an HTTPS base URL. Queries and fragments are rejected.
//
// Key types: [Config].
package meilisearchconfig
