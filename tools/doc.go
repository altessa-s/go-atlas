// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package tools tracks build-time tool dependencies via blank imports.
// It is guarded by a "tools" build tag so these dependencies are recorded
// in go.mod but never compiled into production binaries.
package tools
