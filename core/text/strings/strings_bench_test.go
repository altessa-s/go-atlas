// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package strings_test

import (
	"strings"
	"testing"

	corestrings "github.com/altessa-s/go-atlas/core/text/strings"
)

func BenchmarkJoin(b *testing.B) {
	elements := []string{"a", "b", "c", "", "d"}
	opts := corestrings.JoinOptions{Separator: ",", SkipEmpty: true}

	b.Run("CoreJoin", func(b *testing.B) {
		for b.Loop() {
			corestrings.Join(elements, opts)
		}
	})

	b.Run("StdJoin", func(b *testing.B) {
		for b.Loop() {
			// Simulate skip empty manually for fair comparison or just run simple join
			res := make([]string, 0, len(elements))
			for _, e := range elements {
				if e != "" {
					res = append(res, e)
				}
			}
			strings.Join(res, ",")
		}
	})
}

func BenchmarkSplit(b *testing.B) {
	s := "a,b,c,,d"
	opts := corestrings.SplitOptions{Separator: ",", SkipEmpty: true}

	b.Run("CoreSplit", func(b *testing.B) {
		for b.Loop() {
			corestrings.Split(s, opts)
		}
	})

	b.Run("StdSplit", func(b *testing.B) {
		for b.Loop() {
			parts := strings.Split(s, ",")
			// Simulate skip empty
			res := make([]string, 0, len(parts))
			for _, p := range parts {
				if p != "" {
					res = append(res, p) //nolint:staticcheck // SA4010: benchmark measures split+filter allocation pattern
				}
			}
			_ = res
		}
	})
}

func BenchmarkUnsafe(b *testing.B) {
	s := "hello world big string"

	b.Run("ToBytesUnsafe", func(b *testing.B) {
		for b.Loop() {
			_ = corestrings.ToBytesUnsafe(s)
		}
	})

	b.Run("StdConversion", func(b *testing.B) {
		for b.Loop() {
			_ = []byte(s)
		}
	})
}

func BenchmarkConcat(b *testing.B) {
	s1, s2, s3 := "hello", " ", "world"

	b.Run("Concat", func(b *testing.B) {
		for b.Loop() {
			corestrings.Concat(s1, s2, s3)
		}
	})

	b.Run("ConcatUnsafe", func(b *testing.B) {
		for b.Loop() {
			corestrings.ConcatUnsafe(s1, s2, s3)
		}
	})
}
