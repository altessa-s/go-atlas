// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package freecache

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

// DefaultMaxSize is the default maximum cache size (100MB).
const DefaultMaxSize = 100 * 1024 * 1024

// options contains FreeCache provider configuration.
type options struct {
	maxSize int `optgen:"default=DefaultMaxSize"`
}
