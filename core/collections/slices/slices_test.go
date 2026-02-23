// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package slices_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

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
			if !slices.Equal(got, tt.expected) {
				t.Errorf("Deduplicate() = %v, want %v", got, tt.expected)
			}
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
			if !slices.Equal(got, tt.expected) {
				t.Errorf("Delete() = %v, want %v", got, tt.expected)
			}
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
			input: []testhelpers.Person{{"Alice", 30}, {"Bob", 25}, {"Charlie", 30}},
			keyFn: func(p testhelpers.Person) int { return p.Age },
			expected: map[int][]testhelpers.Person{
				25: {{"Bob", 25}},
				30: {{"Alice", 30}, {"Charlie", 30}},
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
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("GroupBy() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestFilter(t *testing.T) {
	input := []int{1, 2, 3, 4, 5, 6}
	even := func(n int) bool { return n%2 == 0 }

	t.Run("FilterParallel", func(t *testing.T) {
		got := coreslices.FilterParallel(input, even)
		expected := []int{2, 4, 6}
		if !slices.Equal(got, expected) {
			t.Errorf("FilterParallel() = %v, want %v", got, expected)
		}
	})

	t.Run("FilterFirst", func(t *testing.T) {
		val, found := coreslices.FilterFirst(input, even)
		if !found || val != 2 {
			t.Errorf("FilterFirst() = (%v, %v), want (2, true)", val, found)
		}

		_, found = coreslices.FilterFirst(input, func(n int) bool { return n > 10 })
		if found {
			t.Errorf("FilterFirst() found element when none expected")
		}
	})

	t.Run("FilterLast", func(t *testing.T) {
		val, found := coreslices.FilterLast(input, even)
		if !found || val != 6 {
			t.Errorf("FilterLast() = (%v, %v), want (6, true)", val, found)
		}
	})
}

func TestAnyAll(t *testing.T) {
	input := []int{1, 2, 3, 4}

	t.Run("Any", func(t *testing.T) {
		if !coreslices.Any(input, func(n int) bool { return n == 3 }) {
			t.Error("Any() should return true for existing element")
		}
		if coreslices.Any(input, func(n int) bool { return n == 5 }) {
			t.Error("Any() should return false for non-existing element")
		}
	})

	t.Run("All", func(t *testing.T) {
		if !coreslices.All(input, func(n int) bool { return n > 0 }) {
			t.Error("All() should return true when all satisfy")
		}
		if coreslices.All(input, func(n int) bool { return n%2 == 0 }) {
			t.Error("All() should return false when some don't satisfy")
		}
	})
}

func TestAppendHelpers(t *testing.T) {
	t.Run("AppendIf", func(t *testing.T) {
		s := []string{"a"}
		s = coreslices.AppendIf(s, true, "b")
		s = coreslices.AppendIf(s, false, "c")
		expected := []string{"a", "b"}
		if !slices.Equal(s, expected) {
			t.Errorf("AppendIf() = %v, want %v", s, expected)
		}
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
		if !slices.Equal(s, expected) {
			t.Errorf("AppendIfFunc() = %v, want %v", s, expected)
		}
	})

	t.Run("AppendNonEmpty", func(t *testing.T) {
		var s []any
		s = coreslices.AppendNonEmpty(s, "k1", "v1")
		s = coreslices.AppendNonEmpty(s, "k2", "")
		if len(s) != 2 || s[0] != "k1" || s[1] != "v1" {
			t.Errorf("AppendNonEmpty() = %v, want [k1 v1]", s)
		}
	})
}

func TestListConversions(t *testing.T) {
	t.Run("ToList", func(t *testing.T) {
		input := []int{1, 2, 3}
		l := coreslices.ToList(input)
		if l == nil || l.Len() != 3 {
			t.Fatalf("ToList failed: expected length 3, got %d", l.Len())
		}

		i := 0
		for e := l.Front(); e != nil; e = e.Next() {
			if e.Value != input[i] {
				t.Errorf("ToList mismatch at %d: got %v, want %v", i, e.Value, input[i])
			}
			i++
		}

		if coreslices.ToList[int](nil) != nil {
			t.Error("ToList(nil) should return nil")
		}
	})

	t.Run("FromList", func(t *testing.T) {
		input := []int{1, 2, 3}
		l := coreslices.ToList(input)
		got := coreslices.FromList[int](l)
		if !slices.Equal(got, input) {
			t.Errorf("FromList() = %v, want %v", got, input)
		}

		if coreslices.FromList[int](nil) != nil {
			t.Error("FromList(nil) should return nil")
		}
	})
}

func TestAnyConversions(t *testing.T) {
	t.Run("ToAny", func(t *testing.T) {
		input := []int{1, 2}
		got := coreslices.ToAny(input)
		if len(got) != 2 || got[0] != 1 || got[1] != 2 {
			t.Errorf("ToAny() = %v, want [1 2]", got)
		}
	})

	t.Run("ToStrings", func(t *testing.T) {
		input := []any{"a", 1, "b", true}
		got := coreslices.ToStrings(input)
		expected := []string{"a", "b"}
		if !slices.Equal(got, expected) {
			t.Errorf("ToStrings() = %v, want %v", got, expected)
		}
	})
}

func TestReduce(t *testing.T) {
	input := []int{1, 2, 3, 4}
	sum := coreslices.Reduce(input, 0, func(acc, idx, val int) int {
		return acc + val
	})
	if sum != 10 {
		t.Errorf("Reduce() sum = %d, want 10", sum)
	}
}

func TestMapParallel(t *testing.T) {
	input := []int{1, 2, 3}
	got := coreslices.MapParallel(input, func(n int) int {
		return n * 2
	})
	expected := []int{2, 4, 6}
	if !slices.Equal(got, expected) {
		t.Errorf("MapParallel() = %v, want %v", got, expected)
	}
}

func TestToWithFilter(t *testing.T) {
	input := []int{1, 2, 3, 4}
	got := coreslices.ToWithFilter(input,
		func(n int) bool { return n%2 == 0 },
		func(n int) string { return fmt.Sprintf("%d", n) },
	)
	expected := []string{"2", "4"}
	if !slices.Equal(got, expected) {
		t.Errorf("ToWithFilter() = %v, want %v", got, expected)
	}
}

func TestDeduplicateBy(t *testing.T) {
	input := []testhelpers.User{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}, {ID: 1, Name: "A2"}}
	got := coreslices.DeduplicateBy(input, func(p testhelpers.User) int { return p.ID })
	expected := []testhelpers.User{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("DeduplicateBy() = %v, want %v", got, expected)
	}
}

func TestOrdering(t *testing.T) {
	t.Run("StrictlyIncreasing", func(t *testing.T) {
		if !coreslices.IsStrictlyIncreasing([]int{1, 2, 3}) {
			t.Error("1,2,3 should be strictly increasing")
		}
		if coreslices.IsStrictlyIncreasing([]int{1, 1, 2}) {
			t.Error("1,1,2 should NOT be strictly increasing")
		}
	})

	t.Run("StrictlyDecreasing", func(t *testing.T) {
		if !coreslices.IsStrictlyDecreasing([]int{3, 2, 1}) {
			t.Error("3,2,1 should be strictly decreasing")
		}
		if coreslices.IsStrictlyDecreasing([]int{3, 3, 2}) {
			t.Error("3,3,2 should NOT be strictly decreasing")
		}
	})

	t.Run("NonDecreasing", func(t *testing.T) {
		if !coreslices.IsNonDecreasing([]int{1, 2, 2, 3}) {
			t.Error("1,2,2,3 should be non-decreasing")
		}
		if coreslices.IsNonDecreasing([]int{1, 3, 2}) {
			t.Error("1,3,2 should NOT be non-decreasing")
		}
	})

	t.Run("NonIncreasing", func(t *testing.T) {
		if !coreslices.IsNonIncreasing([]int{3, 2, 2, 1}) {
			t.Error("3,2,2,1 should be non-increasing")
		}
		if coreslices.IsNonIncreasing([]int{3, 1, 2}) {
			t.Error("3,1,2 should NOT be non-increasing")
		}
	})
}

func TestAppendNonNil(t *testing.T) {
	s := []*int{}
	i := 1
	s = coreslices.AppendNonNil(s, &i)
	s = coreslices.AppendNonNil(s, nil)
	if len(s) != 1 {
		t.Errorf("AppendNonNil failed len=%d", len(s))
	}
}
