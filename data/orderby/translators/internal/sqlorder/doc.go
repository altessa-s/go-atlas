// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sqlorder renders an orderby.Spec as a SQL ORDER BY body for the
// PostgreSQL, MariaDB and ClickHouse translators, which differ only in
// identifier quoting.
package sqlorder
