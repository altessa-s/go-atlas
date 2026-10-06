// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package maps

import (
	"iter"
)

// Filter returns an [iter.Seq2] iterator that yields only the key-value pairs from m
// for which predicate returns true. Pairs that do not satisfy the predicate are
// skipped without allocation. The iteration order is non-deterministic.
//
// Use [FilterMap] instead when you need a materialized map rather than a lazy iterator.
func Filter[K comparable, V any](m map[K]V, predicate func(K, V) bool) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for k, v := range m {
			if predicate(k, v) {
				if !yield(k, v) {
					return
				}
			}
		}
	}
}

// Map returns an [iter.Seq2] iterator that applies fn to each key-value pair in m,
// yielding the transformed (K2, V2) pairs. Both the key and value types may change
// through the transformation. The iteration order is non-deterministic.
//
// Use [ConvertMap] instead when you need a materialized map rather than a lazy iterator.
func Map[K1 comparable, V1 any, K2 comparable, V2 any](m map[K1]V1, fn func(K1, V1) (K2, V2)) iter.Seq2[K2, V2] {
	return func(yield func(K2, V2) bool) {
		for k, v := range m {
			if !yield(fn(k, v)) {
				return
			}
		}
	}
}
