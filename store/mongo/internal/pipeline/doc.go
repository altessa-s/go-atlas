// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package pipeline renders MongoDB aggregation stages for pagination.
// Callers validate inputs and resolve defaults before rendering.
// BSON documents supplied by callers are referenced, not modified or deep-copied.
//
// # Usage
//
//	stages := pipeline.Offset(filter, sort, projection, 20, 0, true)
//
// CursorItems supplies lookahead and decoration stages. CursorResult shapes
// items and optionally counts the original filter independently of the cursor.
package pipeline
