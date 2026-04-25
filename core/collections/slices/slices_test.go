// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slices_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/internal/testhelpers"

	coreslices "github.com/altessa-s/go-atlas/core/collections/slices"
)

func TestDeduplicate(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		expected []int
	}{
		{
			name:     "empty slice",
			input:    []int{},
			expected: nil,
		},
		{
			name:     "nil slice",
			input:    nil,
			expected: nil,
		},
		{
			name:     "no duplicates",
			input:    []int{1, 2, 3},
			expected: []int{1, 2, 3},
		},
		{
			name:     "consecutive duplicates",
			input:    []int{1, 1, 2, 2, 3, 3},
			expected: []int{1, 2, 3},
		},
		{
			name:     "scattered duplicates",
			input:    []int{1, 2, 3, 1, 2, 3},
			expected: []int{1, 2, 3},
		},
		{
			name:     "all same",
			input:    []int{1, 1, 1},
			expected: []int{1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := coreslices.Deduplicate(tt.input)
			require.True(t, slices.Equal(got, tt.expected), "Deduplicate() = %v, want %v", got, tt.expected)
		})
	}
}

func TestDelete(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		target   string
		expected []string
	}{
		{
			name:     "delete existing",
			input:    []string{"a", "b", "c"},
			target:   "b",
			expected: []string{"a", "c"},
		},
		{
			name:     "delete non-existing",
			input:    []string{"a", "b", "c"},
			target:   "d",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "delete from empty",
			input:    []string{},
			target:   "a",
			expected: []string{},
		},
		{
			name:     "delete first",
			input:    []string{"a", "b"},
			target:   "a",
			expected: []string{"b"},
		},
		{
			name:     "delete last",
			input:    []string{"a", "b"},
			target:   "b",
			expected: []string{"a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := coreslices.Delete(tt.input, tt.target)
			require.True(t, slices.Equal(got, tt.expected), "Delete() = %v, want %v", got, tt.expected)
		})
	}
}

func TestGroupBy(t *testing.T) {
	tests := []struct {
		name     string
		input    []testhelpers.Person
		keyFn    func(testhelpers.Person) int
		expected map[int][]testhelpers.Person
	}{
		{
			name:  "group by age",
			input: []testhelpers.Person{{Name: "Alice", Age: 30}, {Name: "Bob", Age: 25}, {Name: "Charlie", Age: 30}},
			keyFn: func(p testhelpers.Person) int { return p.Age },
			expected: map[int][]testhelpers.Person{
				25: {{Name: "Bob", Age: 25}},
				30: {{Name: "Alice", Age: 30}, {Name: "Charlie", Age: 30}},
			},
		},
		{
			name:     "empty input",
			input:    []testhelpers.Person{},
			keyFn:    func(p testhelpers.Person) int { return p.Age },
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := coreslices.GroupBy(tt.input, tt.keyFn)
			require.True(t, reflect.DeepEqual(got, tt.expected), "GroupBy() = %v, want %v", got, tt.expected)
		})
	}
}

func TestFilter(t *testing.T) {
	input := []int{1, 2, 3, 4, 5, 6}
	even := func(n int) bool { return n%2 == 0 }

	t.Run("FilterParallel", func(t *testing.T) {
		got := coreslices.FilterParallel(input, even)
		expected := []int{2, 4, 6}
		require.True(t, slices.Equal(got, expected), "FilterParallel() = %v, want %v", got, expected)
	})

	t.Run("FilterFirst", func(t *testing.T) {
		val, found := coreslices.FilterFirst(input, even)
		require.True(t, found, "FilterFirst() should find an element")
		require.Equal(t, 2, val)

		_, found = coreslices.FilterFirst(input, func(n int) bool { return n > 10 })
		require.False(t, found, "FilterFirst() found element when none expected")
	})

	t.Run("FilterLast", func(t *testing.T) {
		val, found := coreslices.FilterLast(input, even)
		require.True(t, found, "FilterLast() should find an element")
		require.Equal(t, 6, val)
	})
}

func TestAnyAll(t *testing.T) {
	input := []int{1, 2, 3, 4}

	t.Run("Any", func(t *testing.T) {
		require.True(t, coreslices.Any(input, func(n int) bool { return n == 3 }), "Any() should return true for existing element")
		require.False(t, coreslices.Any(input, func(n int) bool { return n == 5 }), "Any() should return false for non-existing element")
	})

	t.Run("All", func(t *testing.T) {
		require.True(t, coreslices.All(input, func(n int) bool { return n > 0 }), "All() should return true when all satisfy")
		require.False(t, coreslices.All(input, func(n int) bool { return n%2 == 0 }), "All() should return false when some don't satisfy")
	})
}

func TestAppendHelpers(t *testing.T) {
	t.Run("AppendIf", func(t *testing.T) {
		s := []string{"a"}
		s = coreslices.AppendIf(s, true, "b")
		s = coreslices.AppendIf(s, false, "c")
		expected := []string{"a", "b"}
		require.True(t, slices.Equal(s, expected), "AppendIf() = %v, want %v", s, expected)
	})

	t.Run("AppendIfFunc", func(t *testing.T) {
		s := []string{"a"}
		s = coreslices.AppendIfFunc(s, true, func() []string {
			return []string{"b"}
		})
		s = coreslices.AppendIfFunc(s, false, func() []string {
			t.Fatal("fn should not be called when cond is false")
			return []string{"c"}
		})
		expected := []string{"a", "b"}
		require.True(t, slices.Equal(s, expected), "AppendIfFunc() = %v, want %v", s, expected)
	})

	t.Run("AppendNonEmpty", func(t *testing.T) {
		var s []any
		s = coreslices.AppendNonEmpty(s, "k1", "v1")
		s = coreslices.AppendNonEmpty(s, "k2", "")
		require.Len(t, s, 2)
		require.Equal(t, "k1", s[0])
		require.Equal(t, "v1", s[1])
	})
}

func TestListConversions(t *testing.T) {
	t.Run("ToList", func(t *testing.T) {
		input := []int{1, 2, 3}
		l := coreslices.ToList(input)
		require.NotNil(t, l)
		require.Equal(t, 3, l.Len())

		i := 0
		for e := l.Front(); e != nil; e = e.Next() {
			require.Equal(t, input[i], e.Value, "ToList mismatch at %d", i)
			i++
		}

		require.Nil(t, coreslices.ToList[int](nil), "ToList(nil) should return nil")
	})

	t.Run("FromList", func(t *testing.T) {
		input := []int{1, 2, 3}
		l := coreslices.ToList(input)
		got := coreslices.FromList[int](l)
		require.True(t, slices.Equal(got, input), "FromList() = %v, want %v", got, input)

		require.Nil(t, coreslices.FromList[int](nil), "FromList(nil) should return nil")
	})
}

func TestAnyConversions(t *testing.T) {
	t.Run("ToAny", func(t *testing.T) {
		input := []int{1, 2}
		got := coreslices.ToAny(input)
		require.Len(t, got, 2)
		require.Equal(t, 1, got[0])
		require.Equal(t, 2, got[1])
	})

	t.Run("ToStrings", func(t *testing.T) {
		input := []any{"a", 1, "b", true}
		got := coreslices.ToStrings(input)
		expected := []string{"a", "b"}
		require.True(t, slices.Equal(got, expected), "ToStrings() = %v, want %v", got, expected)
	})
}

func TestReduce(t *testing.T) {
	input := []int{1, 2, 3, 4}
	sum := coreslices.Reduce(input, 0, func(acc, idx, val int) int {
		return acc + val
	})
	require.Equal(t, 10, sum)
}

func TestMapParallel(t *testing.T) {
	input := []int{1, 2, 3}
	got := coreslices.MapParallel(input, func(n int) int {
		return n * 2
	})
	expected := []int{2, 4, 6}
	require.True(t, slices.Equal(got, expected), "MapParallel() = %v, want %v", got, expected)
}

func TestToWithFilter(t *testing.T) {
	input := []int{1, 2, 3, 4}
	got := coreslices.ToWithFilter(input,
		func(n int) bool { return n%2 == 0 },
		func(n int) string { return fmt.Sprintf("%d", n) },
	)
	expected := []string{"2", "4"}
	require.True(t, slices.Equal(got, expected), "ToWithFilter() = %v, want %v", got, expected)
}

func TestDeduplicateBy(t *testing.T) {
	input := []testhelpers.User{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}, {ID: 1, Name: "A2"}}
	got := coreslices.DeduplicateBy(input, func(p testhelpers.User) int { return p.ID })
	expected := []testhelpers.User{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}
	require.True(t, reflect.DeepEqual(got, expected), "DeduplicateBy() = %v, want %v", got, expected)
}

func TestOrdering(t *testing.T) {
	t.Run("StrictlyIncreasing", func(t *testing.T) {
		require.True(t, coreslices.IsStrictlyIncreasing([]int{1, 2, 3}), "1,2,3 should be strictly increasing")
		require.False(t, coreslices.IsStrictlyIncreasing([]int{1, 1, 2}), "1,1,2 should NOT be strictly increasing")
	})

	t.Run("StrictlyDecreasing", func(t *testing.T) {
		require.True(t, coreslices.IsStrictlyDecreasing([]int{3, 2, 1}), "3,2,1 should be strictly decreasing")
		require.False(t, coreslices.IsStrictlyDecreasing([]int{3, 3, 2}), "3,3,2 should NOT be strictly decreasing")
	})

	t.Run("NonDecreasing", func(t *testing.T) {
		require.True(t, coreslices.IsNonDecreasing([]int{1, 2, 2, 3}), "1,2,2,3 should be non-decreasing")
		require.False(t, coreslices.IsNonDecreasing([]int{1, 3, 2}), "1,3,2 should NOT be non-decreasing")
	})

	t.Run("NonIncreasing", func(t *testing.T) {
		require.True(t, coreslices.IsNonIncreasing([]int{3, 2, 2, 1}), "3,2,2,1 should be non-increasing")
		require.False(t, coreslices.IsNonIncreasing([]int{3, 1, 2}), "3,1,2 should NOT be non-increasing")
	})
}

func TestAppendNonNil(t *testing.T) {
	s := []*int{}
	i := 1
	s = coreslices.AppendNonNil(s, &i)
	s = coreslices.AppendNonNil(s, nil)
	require.Len(t, s, 1)
}
