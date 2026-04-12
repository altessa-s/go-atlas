// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldtracker_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/fieldtracker"
)

func TestTracker_FieldNameCache_DifferentTagsAcrossTypes(t *testing.T) {
	type A struct {
		ID string `json:"id"`
	}
	type B struct {
		ID string `json:"identifier"`
	}

	tr := fieldtracker.NewTracker()

	changedA := tr.GetChangedFields(&A{ID: "1"}, &A{ID: "2"})
	require.Equal(t, []string{"id"}, changedA)

	changedB := tr.GetChangedFields(&B{ID: "1"}, &B{ID: "2"})
	require.Equal(t, []string{"identifier"}, changedB)
}

func TestTracker_EmbeddedPointerStruct_IsFlattened(t *testing.T) {
	type Base struct {
		UpdatedAt int64 `json:"updated_at"`
	}
	type Person struct {
		*Base
		Name string `json:"name"`
	}

	tr := fieldtracker.NewTracker()

	before := &Person{Base: &Base{UpdatedAt: 1}, Name: "a"}
	after := &Person{Base: &Base{UpdatedAt: 2}, Name: "a"}
	changed := tr.GetChangedFields(before, after)

	require.Equal(t, []string{"updated_at"}, changed)
}

func TestTracker_SliceAndMap_PrimitiveDiffs_ReturnIndexedPaths(t *testing.T) {
	type S struct {
		Nums []int            `json:"nums"`
		Tags map[string]bool  `json:"tags"`
		IDs  map[int64]string `json:"ids"`
	}

	tr := fieldtracker.NewTracker()

	before := &S{
		Nums: []int{1, 2, 3},
		Tags: map[string]bool{"a": true, "b": false},
		IDs:  map[int64]string{1: "x"},
	}
	after := &S{
		Nums: []int{1, 9, 3},
		Tags: map[string]bool{"a": false, "b": false},
		IDs:  map[int64]string{}, // removed key 1
	}

	changed := tr.GetChangedFields(before, after)

	// We don't assert order (maps are randomized); just check membership.
	want := map[string]struct{}{
		"nums[1]": {},
		"tags[a]": {},
		"ids[1]":  {},
	}

	require.Len(t, changed, len(want))

	for _, p := range changed {
		_, ok := want[p]
		require.True(t, ok, "changed contains unexpected path %q; full=%v", p, changed)
	}
}

func TestTracker_Options(t *testing.T) {
	type Data struct {
		Name    string `json:"name"`
		Ignored string `json:"ignored"`
		Nested  struct {
			Val int `json:"val"`
		} `json:"nested"`
	}

	t.Run("IgnoreFields", func(t *testing.T) {
		tr := fieldtracker.NewTracker(fieldtracker.WithIgnoreFields("ignored"))
		before := &Data{Name: "A", Ignored: "A", Nested: struct {
			Val int `json:"val"`
		}{Val: 1}}
		after := &Data{Name: "B", Ignored: "B", Nested: struct {
			Val int `json:"val"`
		}{Val: 1}}

		changed := tr.GetChangedFields(before, after)
		require.Equal(t, []string{"name"}, changed)
	})

	t.Run("MaxDepth", func(t *testing.T) {
		// Test depth limiting.
		// Construct nest such that leaf is at depth > MaxDepth

		// Depth depends on implementation details:
		// Root = 0
		// Nested struct field = 1 (compareStructsFast calls with depth+1)

		tr := fieldtracker.NewTracker(fieldtracker.WithMaxDepth(1))

		// If MaxDepth is 1, then root properties (Depth 0) are checked.
		// Nested properties (Depth 1) are checked?
		// Code says: if depth > maxDepth return.

		// So if MaxDepth=1:
		// Root (0) -> Check fields. "nested" is field.
		// compareStructsFast calls for "nested" with depth=1.
		// 1 > 1 is False. So "nested" is entered.
		// Inside "nested" (Struct), loop fields. "val" is field.
		// fastCompare("val") calls compareStructsFast/compareSliceFast etc with depth+1 = 2.
		// But "val" is Int. fastCompare handles primitives directly.
		// Does fastCompare check depth? NO.
		// fastCompare only checks depth when calling back into compareStructsFast/Slice/Map.

		// So Primitives are always compared if their parent struct was processed?
		// Wait, look at compareStructsFast loop:
		// _ = ft.fastCompare(..., depth+1)

		// If "val" (int) is compared, it returns false (diff).
		// Then it's appended to changed.

		// So MaxDepth limits how deep we traverse STRUCTS/SLICES/MAPS.
		// But if we are at the limit, do we check primitives of that level?
		// Yes, because we are already in the loop of the parent (at valid depth).

		// So to test MaxDepth, we need more nesting.
		type Deep struct {
			L1 struct {
				L2 struct {
					Val int `json:"val"`
				} `json:"l2"`
			} `json:"l1"`
		}

		before := &Deep{}
		after := &Deep{}
		after.L1.L2.Val = 1

		// Depth:
		// Root (0)
		// L1 (1) -> Valid if MaxDepth >= 1
		// L2 (2) -> Valid if MaxDepth >= 2

		// Set MaxDepth = 1.
		// Root (0) OK -> Traverse L1.
		// L1 (1) (compareStructsFast called with 1). 1 > 1 False. OK.
		// Inside L1: Field L2. Call fastCompare(L2, depth=2).
		// L2 is Struct. fastCompare calls compareStructsFast(L2, depth=2).
		// 2 > 1 True. Returns immediately.
		// So L2 is NOT traversed. L2's fields (Val) NOT checked.
		// changes should be empty? OR "l1.l2"?

		// fastCompare returns bool (equal).
		// compareStructsFast returns void!
		// But fastCompare calls compareStructsFast and returns TRUE (equal) if it returned (assumes no change if stopped?).
		// Code:
		// if info != nil ... compareStructsFast(...) return true

		// If compareStructsFast returns early due to depth, it adds NOTHING to `changed`.
		// And fastCompare returns true.
		// So no changes detected at that level.

		changed := tr.GetChangedFields(before, after)
		require.Empty(t, changed, "Expected no changes at depth > 1")

		// Now test with MaxDepth = 2
		tr2 := fieldtracker.NewTracker(fieldtracker.WithMaxDepth(2))
		changed2 := tr2.GetChangedFields(before, after)
		require.Equal(t, []string{"l1.l2.val"}, changed2)
	})
}
