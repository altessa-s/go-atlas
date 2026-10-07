// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package clickhouseconfig defines the ClickHouse connection schema.
//
// Schemas carry yaml and default tags read by
// github.com/altessa-s/go-atlas/config/loader, expose Default* constructors
// where defaults exist, and implement Validate; component factories map them
// to generated options. The infrastructure/clickhouse/factory package opens a
// connection from [Config].
//
// Key types: [Config], [Compression].
package clickhouseconfig
