// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slices_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

func FuzzDeduplicate(f *testing.F) {
	f.Add("a", "b", "a", "c")
	f.Add("1", "2", "3", "1")

	f.Fuzz(func(t *testing.T, s1, s2, s3, s4 string) {
		input := []string{s1, s2, s3, s4}
		output := coreslices.Deduplicate(input)

		// Property 1: No duplicates in output
		seen := make(map[string]bool)
		for _, v := range output {
			assert.False(t, seen[v], "Duplicate found in output: %v", v)
			seen[v] = true
		}

		// Property 2: All elements in output must be in input
		for _, v := range output {
			assert.True(t, slices.Contains(input, v), "Element %v in output not found in input", v)
		}

		// Property 3: Length should be <= input length
		assert.LessOrEqual(t, len(output), len(input), "Output length %d > input length %d", len(output), len(input))
	})
}

func FuzzDelete(f *testing.F) {
	f.Add("hello", "l")
	f.Add("world", "o")

	f.Fuzz(func(t *testing.T, s string, target string) {
		// Convert string to slice for testing Delete
		origInput := strings.Split(s, "")
		// Make a copy for existing input check
		input := make([]string, len(origInput))
		copy(input, origInput)

		output := coreslices.Delete(input, target)

		// Check that if we deleted something, length is less
		if slices.Contains(origInput, target) {
			assert.Equal(t, len(origInput)-1, len(output), "Delete failed to reduce length. Input: %v, Target: %v, Output: %v", origInput, target, output)
		} else {
			assert.Equal(t, len(origInput), len(output), "Delete changed length when target not found. Input: %v, Target: %v", origInput, target)
		}
	})
}
