// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

//go:generate go run github.com/altessa-s/go-atlas/tools/codegen/optgen generate

// DefaultCapacity is the default capacity of the filter.
const DefaultCapacity = 100000

// options contains memory Cuckoo filter storage configuration.
type options struct {
	capacity uint `optgen:"default=DefaultCapacity"`
}
