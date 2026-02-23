// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	testpb "github.com/altessa-s/go-atlas/proto/gen/fieldmasktest/v1"
)

// --- helpers ---

func newTestStruct(kvs map[string]interface{}) *structpb.Struct {
	s, err := structpb.NewStruct(kvs)
	if err != nil {
		panic(err)
	}
	return s
}

func msgWithExtraData() *testpb.UpdateRequest {
	return &testpb.UpdateRequest{
		Id:        "1",
		ExtraData: newTestStruct(map[string]interface{}{"key": "val", "num": 42.0}),
	}
}

// --- Filter ---

func TestFilter_StructAsLeaf_KeptIntact(t *testing.T) {
	msg := msgWithExtraData()
	original := proto.Clone(msg).(*testpb.UpdateRequest)

	mask := fieldmask.FromPaths("extra_data")
	mask.Filter(msg)

	assert.True(t, proto.Equal(original.GetExtraData(), msg.GetExtraData()),
		"Struct value should be kept intact when entire field is in mask")
	assert.Equal(t, "", msg.GetId(), "id should be cleared (not in mask)")
}

func TestFilter_StructNestedMask_FiltersKeys(t *testing.T) {
	msg := msgWithExtraData() // {"key": "val", "num": 42}

	mask := fieldmask.FromPaths("extra_data.key")
	mask.Filter(msg)

	// Only "key" should remain; "num" should be removed.
	require.NotNil(t, msg.GetExtraData())
	assert.NotNil(t, msg.GetExtraData().GetFields()["key"], "key should be kept")
	assert.Nil(t, msg.GetExtraData().GetFields()["num"], "num should be filtered out")
}

func TestFilter_StructNotInMask_Cleared(t *testing.T) {
	msg := msgWithExtraData()

	mask := fieldmask.FromPaths("id")
	mask.Filter(msg)

	assert.Equal(t, "1", msg.GetId())
	assert.Nil(t, msg.GetExtraData(), "extra_data should be cleared when not in mask")
}

func TestFilter_StructDeepNested_FiltersRecursively(t *testing.T) {
	msg := &testpb.UpdateRequest{
		Id: "1",
		ExtraData: newTestStruct(map[string]interface{}{
			"workPeriod": map[string]interface{}{
				"from": "2026-02-18",
				"till": "2026-02-19",
			},
			"amount": 352500.0,
		}),
	}

	// Filter to keep only workPeriod.from
	mask := fieldmask.FromPaths("extra_data.workPeriod.from")
	mask.Filter(msg)

	require.NotNil(t, msg.GetExtraData())
	// "amount" should be removed (not in mask)
	assert.Nil(t, msg.GetExtraData().GetFields()["amount"])
	// "workPeriod" should remain, but only with "from"
	wp := msg.GetExtraData().GetFields()["workPeriod"]
	require.NotNil(t, wp)
	require.NotNil(t, wp.GetStructValue())
	assert.NotNil(t, wp.GetStructValue().GetFields()["from"])
	assert.Nil(t, wp.GetStructValue().GetFields()["till"], "till should be filtered out")
}

// --- Prune ---

func TestPrune_StructAsLeaf_Cleared(t *testing.T) {
	msg := msgWithExtraData()

	mask := fieldmask.FromPaths("extra_data")
	mask.Prune(msg)

	assert.Nil(t, msg.GetExtraData(), "extra_data should be pruned")
	assert.Equal(t, "1", msg.GetId(), "id should remain")
}

func TestPrune_StructNestedMask_PrunesKey(t *testing.T) {
	msg := msgWithExtraData() // {"key": "val", "num": 42}

	mask := fieldmask.FromPaths("extra_data.key")
	mask.Prune(msg)

	require.NotNil(t, msg.GetExtraData())
	assert.Nil(t, msg.GetExtraData().GetFields()["key"], "key should be pruned")
	assert.NotNil(t, msg.GetExtraData().GetFields()["num"], "num should remain")
}

func TestPrune_StructNestedMask_NonExistentKey_NoOp(t *testing.T) {
	msg := msgWithExtraData()
	original := proto.Clone(msg).(*testpb.UpdateRequest)

	// Pruning a key that doesn't exist in the Struct is a no-op.
	mask := fieldmask.FromPaths("extra_data.nonexistent")
	mask.Prune(msg)

	assert.True(t, proto.Equal(original.GetExtraData(), msg.GetExtraData()),
		"Struct should be unchanged when pruning nonexistent key")
}

// --- FromSetFields ---

func TestFromSetFields_StructEnumeratesKeys(t *testing.T) {
	msg := msgWithExtraData() // {"key": "val", "num": 42}

	paths := fieldmask.FromSetFields[[]string](msg)
	slices.Sort(paths)

	assert.Contains(t, paths, "extra_data")
	assert.Contains(t, paths, "extra_data.key", "Struct keys should appear as sub-paths")
	assert.Contains(t, paths, "extra_data.num", "Struct keys should appear as sub-paths")
	// Internal proto fields like "extra_data.fields" must NOT appear.
	for _, p := range paths {
		assert.NotContains(t, p, "extra_data.fields",
			"internal proto structure should not leak into paths")
	}
}

func TestFromMessage_StructEnumeratesKeys(t *testing.T) {
	msg := msgWithExtraData()

	paths := fieldmask.FromMessage[[]string](msg)
	slices.Sort(paths)

	assert.Contains(t, paths, "extra_data")
	assert.Contains(t, paths, "extra_data.key")
	assert.Contains(t, paths, "extra_data.num")
	for _, p := range paths {
		assert.NotContains(t, p, "extra_data.fields",
			"internal proto structure should not leak into paths")
	}
}

// --- Validate ---

func TestValidate_StructLeafPath_Valid(t *testing.T) {
	msg := &testpb.UpdateRequest{}
	mask := fieldmask.FromPaths("extra_data")
	assert.NoError(t, mask.Validate(msg))
}

func TestValidate_StructNestedDynamicPath_Valid(t *testing.T) {
	msg := &testpb.UpdateRequest{}
	// Arbitrary sub-paths should be accepted past a Struct field.
	mask := fieldmask.FromPaths("extra_data.any_key.deeply.nested")
	assert.NoError(t, mask.Validate(msg))
}

// --- ApplyUpdateMask ---

func TestApplyUpdateMask_StructInMask_Kept(t *testing.T) {
	name := "John"
	msg := &testpb.UpdateRequest{
		Id:        "1",
		Name:      &name,
		ExtraData: newTestStruct(map[string]interface{}{"a": "b"}),
	}
	original := proto.Clone(msg.GetExtraData()).(*structpb.Struct)

	mask := fieldmask.FromPaths("name", "extra_data")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	assert.True(t, proto.Equal(original, msg.GetExtraData()),
		"extra_data should be kept when in mask")
	assert.Equal(t, "John", msg.GetName())
	assert.Equal(t, "", msg.GetId(), "id should be cleared (not in mask)")
}

func TestApplyUpdateMask_StructNotSet_CreatesEmpty(t *testing.T) {
	name := "John"
	msg := &testpb.UpdateRequest{
		Id:   "1",
		Name: &name,
	}

	mask := fieldmask.FromPaths("name", "extra_data")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	// extra_data was not set but is in the mask — an empty Struct should be created.
	require.NotNil(t, msg.GetExtraData(), "empty default Struct should be created")
}

// --- Filter with readMask (user's use-case) ---

func TestFilter_StructReadMask_WorkPeriod(t *testing.T) {
	// Simulates the user's exact use-case from the bug report.
	msg := &testpb.UpdateRequest{
		Id: "cf691bdd",
		ExtraData: newTestStruct(map[string]interface{}{
			"type": "PIECEWORK",
			"workPeriod": map[string]interface{}{
				"from": "2026-02-18",
				"till": "2026-02-19",
			},
			"amount":      352500.0,
			"displayName": "Карщик для продовольственного склада",
		}),
	}

	mask := fieldmask.FromPaths("extra_data.workPeriod")
	mask.Filter(msg)

	require.NotNil(t, msg.GetExtraData())
	fields := msg.GetExtraData().GetFields()

	// Only workPeriod should remain.
	assert.Len(t, fields, 1)
	wp := fields["workPeriod"]
	require.NotNil(t, wp)
	require.NotNil(t, wp.GetStructValue())
	assert.Equal(t, "2026-02-18", wp.GetStructValue().GetFields()["from"].GetStringValue())
	assert.Equal(t, "2026-02-19", wp.GetStructValue().GetFields()["till"].GetStringValue())
}

// --- Map of Struct ---

func TestFilter_MapOfStruct_KeysFilterNormally(t *testing.T) {
	msg := &testpb.UpdateRequest{
		Id: "1",
		StructMap: map[string]*structpb.Struct{
			"keep": newTestStruct(map[string]interface{}{"x": 1.0}),
			"drop": newTestStruct(map[string]interface{}{"y": 2.0}),
		},
	}

	mask := fieldmask.FromPaths("struct_map.keep")
	mask.Filter(msg)

	require.Len(t, msg.GetStructMap(), 1)
	assert.NotNil(t, msg.GetStructMap()["keep"])
	assert.Nil(t, msg.GetStructMap()["drop"])
}

func TestFilter_MapOfStruct_ValueFiltered(t *testing.T) {
	msg := &testpb.UpdateRequest{
		Id: "1",
		StructMap: map[string]*structpb.Struct{
			"config": newTestStruct(map[string]interface{}{"a": 1.0, "b": 2.0}),
		},
	}

	// Keep only key "a" inside the Struct at map entry "config".
	mask := fieldmask.FromPaths("struct_map.config.a")
	mask.Filter(msg)

	require.Len(t, msg.GetStructMap(), 1)
	s := msg.GetStructMap()["config"]
	require.NotNil(t, s)
	assert.NotNil(t, s.GetFields()["a"], "a should be kept")
	assert.Nil(t, s.GetFields()["b"], "b should be filtered out")
}

func TestPrune_MapOfStruct_KeysPruneNormally(t *testing.T) {
	msg := &testpb.UpdateRequest{
		Id: "1",
		StructMap: map[string]*structpb.Struct{
			"keep": newTestStruct(map[string]interface{}{"x": 1.0}),
			"drop": newTestStruct(map[string]interface{}{"y": 2.0}),
		},
	}

	mask := fieldmask.FromPaths("struct_map.drop")
	mask.Prune(msg)

	require.Len(t, msg.GetStructMap(), 1)
	assert.NotNil(t, msg.GetStructMap()["keep"])
}

func TestFromSetFields_MapOfStruct_EnumeratesKeys(t *testing.T) {
	msg := &testpb.UpdateRequest{
		StructMap: map[string]*structpb.Struct{
			"a": newTestStruct(map[string]interface{}{"k": "v"}),
		},
	}

	paths := fieldmask.FromSetFields[[]string](msg)
	slices.Sort(paths)

	assert.Contains(t, paths, "struct_map")
	assert.Contains(t, paths, "struct_map.a")
	assert.Contains(t, paths, "struct_map.a.k", "Struct keys inside map values should be enumerated")
	// Must NOT recurse into Struct's internal proto fields.
	for _, p := range paths {
		assert.NotContains(t, p, "struct_map.a.fields",
			"should not recurse into internal proto structure")
	}
}

// --- Repeated Struct ---

func TestFilter_RepeatedStruct_KeptAsIs(t *testing.T) {
	msg := &testpb.UpdateRequest{
		Id: "1",
		StructList: []*structpb.Struct{
			newTestStruct(map[string]interface{}{"a": 1.0}),
			newTestStruct(map[string]interface{}{"b": 2.0}),
		},
	}

	mask := fieldmask.FromPaths("struct_list")
	mask.Filter(msg)

	require.Len(t, msg.GetStructList(), 2)
}

func TestPrune_RepeatedStruct_Cleared(t *testing.T) {
	msg := &testpb.UpdateRequest{
		Id: "1",
		StructList: []*structpb.Struct{
			newTestStruct(map[string]interface{}{"a": 1.0}),
		},
	}

	mask := fieldmask.FromPaths("struct_list")
	mask.Prune(msg)

	assert.Empty(t, msg.GetStructList())
	assert.Equal(t, "1", msg.GetId())
}

func TestFromSetFields_RepeatedStruct_EnumeratesKeys(t *testing.T) {
	msg := &testpb.UpdateRequest{
		StructList: []*structpb.Struct{
			newTestStruct(map[string]interface{}{"k": "v"}),
		},
	}

	paths := fieldmask.FromSetFields[[]string](msg)

	assert.Contains(t, paths, "struct_list")
	assert.Contains(t, paths, "struct_list.0")
	assert.Contains(t, paths, "struct_list.0.k", "Struct keys in list elements should be enumerated")
	// Must NOT recurse into Struct's internal proto fields.
	for _, p := range paths {
		assert.NotContains(t, p, "struct_list.0.fields",
			"should not recurse into internal proto structure")
	}
}

func TestValidate_MapOfStruct_Valid(t *testing.T) {
	msg := &testpb.UpdateRequest{}
	mask := fieldmask.FromPaths("struct_map.any_key")
	assert.NoError(t, mask.Validate(msg))
}

func TestValidate_RepeatedStruct_Valid(t *testing.T) {
	msg := &testpb.UpdateRequest{}
	mask := fieldmask.FromPaths("struct_list")
	assert.NoError(t, mask.Validate(msg))
}

// --- ListValue helpers ---

func newTestListValue(items ...interface{}) *structpb.ListValue {
	lv, err := structpb.NewList(items)
	if err != nil {
		panic(err)
	}
	return lv
}

func msgWithListData() *testpb.UpdateRequest {
	return &testpb.UpdateRequest{
		Id: "1",
		ListData: newTestListValue(
			map[string]interface{}{"name": "Alice", "age": 30.0},
			map[string]interface{}{"name": "Bob", "age": 25.0},
		),
	}
}

// --- ListValue Filter ---

func TestFilter_ListValueAsLeaf_KeptIntact(t *testing.T) {
	msg := msgWithListData()
	original := proto.Clone(msg).(*testpb.UpdateRequest)

	mask := fieldmask.FromPaths("list_data")
	mask.Filter(msg)

	assert.True(t, proto.Equal(original.GetListData(), msg.GetListData()),
		"ListValue should be kept intact when entire field is in mask")
	assert.Equal(t, "", msg.GetId(), "id should be cleared (not in mask)")
}

func TestFilter_ListValueNotInMask_Cleared(t *testing.T) {
	msg := msgWithListData()

	mask := fieldmask.FromPaths("id")
	mask.Filter(msg)

	assert.Equal(t, "1", msg.GetId())
	assert.Nil(t, msg.GetListData(), "list_data should be cleared when not in mask")
}

func TestFilter_ListValueNestedMask_FiltersStructElements(t *testing.T) {
	msg := msgWithListData() // [{name: Alice, age: 30}, {name: Bob, age: 25}]

	mask := fieldmask.FromPaths("list_data.name")
	mask.Filter(msg)

	require.NotNil(t, msg.GetListData())
	for _, v := range msg.GetListData().GetValues() {
		s := v.GetStructValue()
		require.NotNil(t, s, "each element should still be a Struct")
		assert.NotNil(t, s.GetFields()["name"], "name should be kept")
		assert.Nil(t, s.GetFields()["age"], "age should be filtered out")
	}
}

func TestFilter_ListValueDeeplyNested(t *testing.T) {
	// ListValue containing Structs with nested ListValues.
	msg := &testpb.UpdateRequest{
		Id: "1",
		ListData: newTestListValue(
			map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"x": 1.0, "y": 2.0},
				},
				"other": "val",
			},
		),
	}

	// Keep only "items" at the top level of each Struct element.
	mask := fieldmask.FromPaths("list_data.items")
	mask.Filter(msg)

	require.NotNil(t, msg.GetListData())
	elem := msg.GetListData().GetValues()[0].GetStructValue()
	require.NotNil(t, elem)
	assert.NotNil(t, elem.GetFields()["items"], "items should be kept")
	assert.Nil(t, elem.GetFields()["other"], "other should be filtered out")
}

// --- ListValue Prune ---

func TestPrune_ListValueAsLeaf_Cleared(t *testing.T) {
	msg := msgWithListData()

	mask := fieldmask.FromPaths("list_data")
	mask.Prune(msg)

	assert.Nil(t, msg.GetListData(), "list_data should be pruned")
	assert.Equal(t, "1", msg.GetId(), "id should remain")
}

func TestPrune_ListValueNestedMask_PrunesKeyFromElements(t *testing.T) {
	msg := msgWithListData() // [{name: Alice, age: 30}, {name: Bob, age: 25}]

	mask := fieldmask.FromPaths("list_data.name")
	mask.Prune(msg)

	require.NotNil(t, msg.GetListData())
	for _, v := range msg.GetListData().GetValues() {
		s := v.GetStructValue()
		require.NotNil(t, s)
		assert.Nil(t, s.GetFields()["name"], "name should be pruned")
		assert.NotNil(t, s.GetFields()["age"], "age should remain")
	}
}

// --- ListValue FromSetFields ---

func TestFromSetFields_ListValueEnumeratesElements(t *testing.T) {
	msg := &testpb.UpdateRequest{
		ListData: newTestListValue(
			map[string]interface{}{"k": "v"},
		),
	}

	paths := fieldmask.FromSetFields[[]string](msg)

	assert.Contains(t, paths, "list_data")
	assert.Contains(t, paths, "list_data.0")
	assert.Contains(t, paths, "list_data.0.k", "Struct keys in ListValue elements should be enumerated")
	// Must NOT recurse into ListValue's internal proto fields.
	for _, p := range paths {
		assert.NotContains(t, p, "list_data.values",
			"internal proto structure should not leak into paths")
	}
}

// --- ListValue Validate ---

func TestValidate_ListValueLeafPath_Valid(t *testing.T) {
	msg := &testpb.UpdateRequest{}
	mask := fieldmask.FromPaths("list_data")
	assert.NoError(t, mask.Validate(msg))
}

func TestValidate_ListValueNestedDynamicPath_Valid(t *testing.T) {
	msg := &testpb.UpdateRequest{}
	// Arbitrary sub-paths should be accepted past a ListValue field.
	mask := fieldmask.FromPaths("list_data.any_key.deeply.nested")
	assert.NoError(t, mask.Validate(msg))
}

// --- Struct containing ListValue ---

func TestFilter_StructContainingListValue(t *testing.T) {
	// extra_data is a Struct with a "items" key that holds a ListValue of Structs.
	msg := &testpb.UpdateRequest{
		Id: "1",
		ExtraData: newTestStruct(map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"name": "A", "score": 1.0},
				map[string]interface{}{"name": "B", "score": 2.0},
			},
			"title": "test",
		}),
	}

	// Keep only "items" and within each item keep only "name".
	mask := fieldmask.FromPaths("extra_data.items.name")
	mask.Filter(msg)

	require.NotNil(t, msg.GetExtraData())
	assert.Nil(t, msg.GetExtraData().GetFields()["title"], "title should be filtered out")

	itemsVal := msg.GetExtraData().GetFields()["items"]
	require.NotNil(t, itemsVal)
	lv := itemsVal.GetListValue()
	require.NotNil(t, lv)

	for _, v := range lv.GetValues() {
		s := v.GetStructValue()
		require.NotNil(t, s)
		assert.NotNil(t, s.GetFields()["name"], "name should be kept")
		assert.Nil(t, s.GetFields()["score"], "score should be filtered out")
	}
}

func TestPrune_StructContainingListValue(t *testing.T) {
	msg := &testpb.UpdateRequest{
		Id: "1",
		ExtraData: newTestStruct(map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"name": "A", "score": 1.0},
			},
			"title": "test",
		}),
	}

	// Prune "name" from each item in the "items" ListValue.
	mask := fieldmask.FromPaths("extra_data.items.name")
	mask.Prune(msg)

	require.NotNil(t, msg.GetExtraData())
	assert.NotNil(t, msg.GetExtraData().GetFields()["title"], "title should remain")

	itemsVal := msg.GetExtraData().GetFields()["items"]
	require.NotNil(t, itemsVal)
	lv := itemsVal.GetListValue()
	require.NotNil(t, lv)

	for _, v := range lv.GetValues() {
		s := v.GetStructValue()
		require.NotNil(t, s)
		assert.Nil(t, s.GetFields()["name"], "name should be pruned")
		assert.NotNil(t, s.GetFields()["score"], "score should remain")
	}
}

// --- ApplyUpdateMask with ListValue ---

func TestApplyUpdateMask_ListValueInMask_Kept(t *testing.T) {
	name := "John"
	msg := &testpb.UpdateRequest{
		Id:       "1",
		Name:     &name,
		ListData: newTestListValue(map[string]interface{}{"a": "b"}),
	}
	original := proto.Clone(msg.GetListData()).(*structpb.ListValue)

	mask := fieldmask.FromPaths("name", "list_data")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	assert.True(t, proto.Equal(original, msg.GetListData()),
		"list_data should be kept when in mask")
	assert.Equal(t, "", msg.GetId(), "id should be cleared (not in mask)")
}

func TestApplyUpdateMask_ListValueNotSet_CreatesEmpty(t *testing.T) {
	name := "John"
	msg := &testpb.UpdateRequest{
		Id:   "1",
		Name: &name,
	}

	mask := fieldmask.FromPaths("name", "list_data")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	// list_data was not set but is in the mask — an empty ListValue should be created.
	require.NotNil(t, msg.GetListData(), "empty default ListValue should be created")
}

// --- ListValue with scalar elements (no-op) ---

func TestFilter_ListValueScalarElements_Unaffected(t *testing.T) {
	// ListValue with scalar (non-Struct) elements — nested mask should not crash.
	msg := &testpb.UpdateRequest{
		Id:       "1",
		ListData: newTestListValue("hello", 42.0, true),
	}
	original := proto.Clone(msg).(*testpb.UpdateRequest)

	// Nested mask path: since elements are scalars, they should be unaffected.
	mask := fieldmask.FromPaths("list_data.anything")
	mask.Filter(msg)

	assert.True(t, proto.Equal(original.GetListData(), msg.GetListData()),
		"scalar elements should be unaffected by nested mask")
}

// --- FromSetFields with nested ListValue ---

func TestFromSetFields_StructWithListValue(t *testing.T) {
	msg := &testpb.UpdateRequest{
		ExtraData: newTestStruct(map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"x": 1.0},
			},
		}),
	}

	paths := fieldmask.FromSetFields[[]string](msg)

	assert.Contains(t, paths, "extra_data")
	assert.Contains(t, paths, "extra_data.items")
	assert.Contains(t, paths, "extra_data.items.0")
	assert.Contains(t, paths, "extra_data.items.0.x")
}

// --- Value helpers ---

func newTestValue(v interface{}) *structpb.Value {
	val, err := structpb.NewValue(v)
	if err != nil {
		panic(err)
	}
	return val
}

func msgWithValueData() *testpb.UpdateRequest {
	return &testpb.UpdateRequest{
		Id:        "1",
		ValueData: newTestValue(map[string]interface{}{"key": "val", "num": 42.0}),
	}
}

// --- Value Filter ---

func TestFilter_ValueAsLeaf_KeptIntact(t *testing.T) {
	msg := msgWithValueData()
	original := proto.Clone(msg).(*testpb.UpdateRequest)

	mask := fieldmask.FromPaths("value_data")
	mask.Filter(msg)

	assert.True(t, proto.Equal(original.GetValueData(), msg.GetValueData()),
		"Value should be kept intact when entire field is in mask")
	assert.Equal(t, "", msg.GetId(), "id should be cleared (not in mask)")
}

func TestFilter_ValueNotInMask_Cleared(t *testing.T) {
	msg := msgWithValueData()

	mask := fieldmask.FromPaths("id")
	mask.Filter(msg)

	assert.Equal(t, "1", msg.GetId())
	assert.Nil(t, msg.GetValueData(), "value_data should be cleared when not in mask")
}

func TestFilter_ValueNestedStruct_FiltersKeys(t *testing.T) {
	msg := msgWithValueData() // Value wrapping {"key": "val", "num": 42}

	mask := fieldmask.FromPaths("value_data.key")
	mask.Filter(msg)

	require.NotNil(t, msg.GetValueData())
	sv := msg.GetValueData().GetStructValue()
	require.NotNil(t, sv, "Value should still hold a Struct")
	assert.NotNil(t, sv.GetFields()["key"], "key should be kept")
	assert.Nil(t, sv.GetFields()["num"], "num should be filtered out")
}

func TestFilter_ValueNestedListValue_FiltersStructElements(t *testing.T) {
	// Value wrapping a list of Structs.
	msg := &testpb.UpdateRequest{
		Id: "1",
		ValueData: newTestValue([]interface{}{
			map[string]interface{}{"name": "Alice", "age": 30.0},
			map[string]interface{}{"name": "Bob", "age": 25.0},
		}),
	}

	mask := fieldmask.FromPaths("value_data.name")
	mask.Filter(msg)

	require.NotNil(t, msg.GetValueData())
	lv := msg.GetValueData().GetListValue()
	require.NotNil(t, lv, "Value should still hold a ListValue")

	for _, v := range lv.GetValues() {
		s := v.GetStructValue()
		require.NotNil(t, s)
		assert.NotNil(t, s.GetFields()["name"], "name should be kept")
		assert.Nil(t, s.GetFields()["age"], "age should be filtered out")
	}
}

// --- Value Prune ---

func TestPrune_ValueAsLeaf_Cleared(t *testing.T) {
	msg := msgWithValueData()

	mask := fieldmask.FromPaths("value_data")
	mask.Prune(msg)

	assert.Nil(t, msg.GetValueData(), "value_data should be pruned")
	assert.Equal(t, "1", msg.GetId(), "id should remain")
}

func TestPrune_ValueNestedStruct_PrunesKey(t *testing.T) {
	msg := msgWithValueData() // Value wrapping {"key": "val", "num": 42}

	mask := fieldmask.FromPaths("value_data.key")
	mask.Prune(msg)

	require.NotNil(t, msg.GetValueData())
	sv := msg.GetValueData().GetStructValue()
	require.NotNil(t, sv)
	assert.Nil(t, sv.GetFields()["key"], "key should be pruned")
	assert.NotNil(t, sv.GetFields()["num"], "num should remain")
}

// --- Value FromSetFields ---

func TestFromSetFields_ValueWithStruct_EnumeratesKeys(t *testing.T) {
	msg := msgWithValueData() // Value wrapping {"key": "val", "num": 42}

	paths := fieldmask.FromSetFields[[]string](msg)
	slices.Sort(paths)

	assert.Contains(t, paths, "value_data")
	assert.Contains(t, paths, "value_data.key", "Struct keys inside Value should appear as sub-paths")
	assert.Contains(t, paths, "value_data.num", "Struct keys inside Value should appear as sub-paths")
	// Internal proto fields like "value_data.kind" must NOT appear.
	for _, p := range paths {
		assert.NotContains(t, p, "value_data.kind",
			"internal proto structure should not leak into paths")
		assert.NotContains(t, p, "value_data.struct_value",
			"internal proto structure should not leak into paths")
	}
}

func TestFromSetFields_ValueWithScalar_OnlyLeafPath(t *testing.T) {
	msg := &testpb.UpdateRequest{
		ValueData: newTestValue("hello"),
	}

	paths := fieldmask.FromSetFields[[]string](msg)

	assert.Contains(t, paths, "value_data")
	// Scalar Value should produce only the leaf path, no sub-paths.
	for _, p := range paths {
		if p == "value_data" {
			continue
		}
		assert.False(t, len(p) > len("value_data") && p[:len("value_data")+1] == "value_data.",
			"scalar Value should not produce sub-paths, got: %s", p)
	}
}

// --- Value Validate ---

func TestValidate_ValueLeafPath_Valid(t *testing.T) {
	msg := &testpb.UpdateRequest{}
	mask := fieldmask.FromPaths("value_data")
	assert.NoError(t, mask.Validate(msg))
}

func TestValidate_ValueNestedDynamicPath_Valid(t *testing.T) {
	msg := &testpb.UpdateRequest{}
	// Arbitrary sub-paths should be accepted past a Value field.
	mask := fieldmask.FromPaths("value_data.any.deep.path")
	assert.NoError(t, mask.Validate(msg))
}

// --- Value ApplyUpdateMask ---

func TestApplyUpdateMask_ValueInMask_Kept(t *testing.T) {
	name := "John"
	msg := &testpb.UpdateRequest{
		Id:        "1",
		Name:      &name,
		ValueData: newTestValue(map[string]interface{}{"a": "b"}),
	}
	original := proto.Clone(msg.GetValueData()).(*structpb.Value)

	mask := fieldmask.FromPaths("name", "value_data")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	assert.True(t, proto.Equal(original, msg.GetValueData()),
		"value_data should be kept when in mask")
	assert.Equal(t, "", msg.GetId(), "id should be cleared (not in mask)")
}

func TestApplyUpdateMask_ValueNotSet_CreatesEmpty(t *testing.T) {
	name := "John"
	msg := &testpb.UpdateRequest{
		Id:   "1",
		Name: &name,
	}

	mask := fieldmask.FromPaths("name", "value_data")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	// value_data was not set but is in the mask — an empty Value should be created.
	require.NotNil(t, msg.GetValueData(), "empty default Value should be created")
}

// --- Scalar Value with nested mask (no crash, kept as-is) ---

func TestFilter_ScalarValueWithNestedMask_KeptAsIs(t *testing.T) {
	msg := &testpb.UpdateRequest{
		Id:        "1",
		ValueData: newTestValue(42.0),
	}
	original := proto.Clone(msg).(*testpb.UpdateRequest)

	// Nested mask on a scalar Value: should not crash, value kept as-is.
	mask := fieldmask.FromPaths("value_data.anything")
	mask.Filter(msg)

	assert.True(t, proto.Equal(original.GetValueData(), msg.GetValueData()),
		"scalar Value should be unaffected by nested mask")
}

func TestPrune_ScalarValueWithNestedMask_KeptAsIs(t *testing.T) {
	msg := &testpb.UpdateRequest{
		Id:        "1",
		ValueData: newTestValue("hello"),
	}
	original := proto.Clone(msg).(*testpb.UpdateRequest)

	// Nested mask on a scalar Value: should not crash, value kept as-is.
	mask := fieldmask.FromPaths("value_data.anything")
	mask.Prune(msg)

	assert.True(t, proto.Equal(original.GetValueData(), msg.GetValueData()),
		"scalar Value should be unaffected by nested prune mask")
}
