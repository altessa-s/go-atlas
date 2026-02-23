// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slices

import (
	"container/list"
	"iter"
)

// List returns an [iter.Seq] iterator that yields elements from a [container/list.List],
// performing a type assertion to T for each element. Elements that cannot be asserted
// to T are silently skipped. If l is nil, an empty iterator is returned.
//
// Example:
//
//	l := list.New()
//	l.PushBack(1)
//	l.PushBack("two")
//	for n := range slices.List[int](l) {
//	    fmt.Println(n) // prints 1
//	}
func List[T any](l *list.List) iter.Seq[T] {
	if l == nil {
		return func(yield func(T) bool) {}
	}
	return func(yield func(T) bool) {
		for e := l.Front(); e != nil; e = e.Next() {
			if v, ok := e.Value.(T); ok {
				if !yield(v) {
					return
				}
			}
		}
	}
}

// Filter returns an [iter.Seq] iterator that yields only the elements of collection
// for which predicate returns true. Elements are evaluated lazily during iteration,
// so no intermediate slice is allocated.
// Use slices.Collect() to materialize the result into a slice.
//
// Example:
//
//	nums := []int{1, 2, 3, 4, 5}
//	for n := range slices.Filter(nums, func(n int) bool { return n%2 == 0 }) {
//	    fmt.Println(n) // prints 2, 4
//	}
//
//	// To get a slice:
//	evens := slices.Collect(slices.Filter(nums, isEven))
func Filter[T any](collection []T, predicate func(T) bool) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, v := range collection {
			if predicate(v) {
				if !yield(v) {
					return
				}
			}
		}
	}
}

// Map returns an [iter.Seq] iterator that applies fn to each element of collection,
// yielding the transformed values. Elements are transformed lazily during iteration,
// so no intermediate slice is allocated.
//
// Example:
//
//	nums := []int{1, 2, 3}
//	it := slices.Map(nums, func(n int) string { return fmt.Sprintf("num:%d", n) })
//	for s := range it {
//	    fmt.Println(s) // prints num:1, num:2, num:3
//	}
func Map[T any, O any](collection []T, fn func(T) O) iter.Seq[O] {
	return func(yield func(O) bool) {
		for _, v := range collection {
			if !yield(fn(v)) {
				return
			}
		}
	}
}

// Chunk returns an [iter.Seq] iterator that yields successive sub-slices of collection,
// each of length size, except possibly the last chunk which may be shorter. Each yielded
// slice shares the underlying array with collection. If size is less than 1, an empty
// iterator is returned.
// Use slices.Collect() to materialize the result into a slice of slices.
//
// Example:
//
//	nums := []int{1, 2, 3, 4, 5}
//	for chunk := range slices.Chunk(nums, 2) {
//	    fmt.Println(chunk) // prints [1 2], [3 4], [5]
//	}
//
//	// To get a slice of slices:
//	chunks := slices.Collect(slices.Chunk(nums, 2))
func Chunk[T any](collection []T, size int) iter.Seq[[]T] {
	if size < 1 {
		return func(yield func([]T) bool) {}
	}
	return func(yield func([]T) bool) {
		for i := 0; i < len(collection); i += size {
			end := min(i+size, len(collection))
			if !yield(collection[i:end]) {
				return
			}
		}
	}
}

// Values returns an [iter.Seq] iterator that yields every element of collection in
// order. This is functionally equivalent to a range loop but provides the [iter.Seq]
// interface for composing with other iterator combinators like [FilterSeq] and [MapSeq].
//
// Example:
//
//	nums := []int{1, 2, 3, 4, 5}
//	for n := range slices.Values(nums) {
//	    fmt.Println(n) // prints 1, 2, 3, 4, 5
//	}
func Values[T any](collection []T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, v := range collection {
			if !yield(v) {
				return
			}
		}
	}
}

// Backward returns an [iter.Seq2] iterator that yields (index, element) pairs from
// collection in reverse order, starting from the last element. This is useful for
// reverse iteration without manually managing index arithmetic.
func Backward[T any](collection []T) iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		for i := len(collection) - 1; i >= 0; i-- {
			if !yield(i, collection[i]) {
				return
			}
		}
	}
}

// FilterSeq returns an [iter.Seq] iterator that yields only the elements from seq
// for which predicate returns true. Unlike [Filter], which operates on a concrete
// slice, FilterSeq composes with any [iter.Seq] source.
//
// Example:
//
//	nums := slices.Values([]int{1, 2, 3, 4, 5})
//	for n := range slices.FilterSeq(nums, func(n int) bool { return n%2 == 0 }) {
//	    fmt.Println(n) // prints 2, 4
//	}
func FilterSeq[T any](seq iter.Seq[T], predicate func(T) bool) iter.Seq[T] {
	return func(yield func(T) bool) {
		for v := range seq {
			if predicate(v) {
				if !yield(v) {
					return
				}
			}
		}
	}
}

// MapSeq returns an [iter.Seq] iterator that applies fn to each element yielded by
// seq, producing transformed values of type O. Unlike [Map], which operates on a
// concrete slice, MapSeq composes with any [iter.Seq] source.
//
// Example:
//
//	nums := slices.Values([]int{1, 2, 3})
//	for s := range slices.MapSeq(nums, func(n int) string { return fmt.Sprintf("num:%d", n) }) {
//	    fmt.Println(s) // prints num:1, num:2, num:3
//	}
func MapSeq[T, O any](seq iter.Seq[T], fn func(T) O) iter.Seq[O] {
	return func(yield func(O) bool) {
		for v := range seq {
			if !yield(fn(v)) {
				return
			}
		}
	}
}

// Take returns an [iter.Seq] iterator that yields at most n elements from seq, then
// stops. If seq produces fewer than n elements, all of them are yielded. A non-positive
// n produces an empty iterator.
//
// Example:
//
//	nums := slices.Values([]int{1, 2, 3, 4, 5})
//	for n := range slices.Take(nums, 3) {
//	    fmt.Println(n) // prints 1, 2, 3
//	}
func Take[T any](seq iter.Seq[T], n int) iter.Seq[T] {
	return func(yield func(T) bool) {
		count := 0
		for v := range seq {
			if count >= n {
				return
			}
			if !yield(v) {
				return
			}
			count++
		}
	}
}
