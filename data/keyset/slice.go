// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package keyset

import (
	"slices"

	"github.com/altessa-s/go-atlas/core/types/constraints"
)

// PageSize resolves a requested page size: a value of zero or less is def,
// and the result is clamped to [1, maxSize]. A maxSize below 1 is treated as
// 1, so the result is always positive. A caller that must reject a negative
// request checks it before.
func PageSize[N constraints.Integer](requested N, def, maxSize int) int {
	maxSize = max(maxSize, 1)
	n := def
	if requested > 0 {
		// Compare before converting: a requested value beyond the int range
		// (an uint64 on a 32-bit platform) is still clamped to maxSize.
		if uint64(requested) > uint64(maxSize) { //nolint:gosec // requested > 0
			return maxSize
		}
		n = int(requested)
	}
	return min(max(n, 1), maxSize)
}

// SlicePage is one page of a [SlicePager] read.
type SlicePage[T any] struct {
	// Items are the items of the page, a subslice of the input.
	Items []T
	// Next is the token of the following page, empty on the last page.
	Next string
}

// SlicePager pages a slice held in memory — a small catalog or configuration
// collection — with signed page tokens. Build it with [NewSlicePager]
// (keyset position: an item added or removed between pages neither repeats
// nor skips the others) or [NewSliceOffsetPager] (position by index). It is
// safe for concurrent use.
type SlicePager[T any] struct {
	// start returns the index of the first item of the page token selects.
	start func(items []T, token string, b Bindings) (int, error)
	// next returns the token of the page after items[:end].
	next func(items []T, end int, b Bindings) (string, error)
}

// NewSlicePager returns a keyset pager: a token carries key of the last item
// of its page, and the next page starts at the first item whose key compares
// greater than it. Items must be ordered by compare on key, ascending, and
// keys must be unique.
func NewSlicePager[T, C any](tokens Typed[C], key func(T) C, compare func(a, b C) int) SlicePager[T] {
	return SlicePager[T]{
		start: func(items []T, token string, b Bindings) (int, error) {
			after, first, err := tokens.Resolve(token, b)
			if err != nil || first {
				return 0, err
			}
			i := slices.IndexFunc(items, func(it T) bool { return compare(key(it), after) > 0 })
			if i < 0 {
				return len(items), nil
			}
			return i, nil
		},
		next: func(items []T, end int, b Bindings) (string, error) {
			return tokens.Issue(key(items[end-1]), b)
		},
	}
}

// NewSliceOffsetPager returns an offset pager: a token carries the index of
// the first item of its page. An offset beyond the slice — the catalog shrank
// since the token was issued — fails with [ErrInvalidToken].
func NewSliceOffsetPager[T any](tokens Offset) SlicePager[T] {
	return SlicePager[T]{
		start: func(items []T, token string, b Bindings) (int, error) {
			offset, err := tokens.Resolve(token, b)
			if err != nil {
				return 0, err
			}
			if offset > len(items) {
				return 0, ErrInvalidToken
			}
			return offset, nil
		},
		next: func(_ []T, end int, b Bindings) (string, error) {
			return tokens.Issue(end, b)
		},
	}
}

// Page returns the page of items that token selects, at most size items
// (resolve the request with [PageSize]; a size below 1 is treated as 1), and
// the token of the next page unless the page reaches the end of items. A
// token failure returns the keyset error, classified by [Classify].
func (p SlicePager[T]) Page(items []T, size int, token string, b Bindings) (SlicePage[T], error) {
	start, err := p.start(items, token, b)
	if err != nil {
		return SlicePage[T]{}, err
	}
	end := start + min(max(size, 1), len(items)-start)
	page := SlicePage[T]{Items: items[start:end]}
	if end < len(items) {
		if page.Next, err = p.next(items, end, b); err != nil {
			return SlicePage[T]{}, err
		}
	}
	return page, nil
}
