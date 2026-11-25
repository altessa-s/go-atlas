// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package txscope coordinates context lifetimes across transaction adapters
// and the unit of work. It tracks nested savepoints and identifies metadata
// that should be hidden from a callback after commit.
package txscope
