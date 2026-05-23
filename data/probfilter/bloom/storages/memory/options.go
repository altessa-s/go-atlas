// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package memory

//go:generate go run github.com/altessa-s/go-atlas/cmd/optgen generate

// DefaultExpectedItems is the default expected number of items.
const DefaultExpectedItems = 100000

// DefaultFalsePositiveRate is the default target false positive rate.
const DefaultFalsePositiveRate = 0.01

// options contains memory Bloom filter storage configuration.
type options struct {
	expectedItems     int64   `optgen:"default=DefaultExpectedItems" optval:"positive"`
	falsePositiveRate float64 `optgen:"default=DefaultFalsePositiveRate" optval:"positive=allow_zero"`
}
