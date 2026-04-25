// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package validation provides lightweight, allocation-free validators
// shared across the transport layer. Currently contains [IsValidUUIDv4],
// a hand-rolled UUID v4 check that avoids regexp overhead on hot paths.
package validation
