// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package timeformat provides time formatting with RFC3339 and Unix timestamp support.
//
// Example:
//
//	formatted := timeformat.RFC3339.Format(time.Now())
//	parsed, _ := timeformat.UnixMilli.Parse("1704110400123")
package timeformat
