// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldmask_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/proto/fieldmask"

	"google.golang.org/protobuf/proto"

	testpb "github.com/altessa-s/go-atlas/proto/gen/fieldmasktest/v1"
)

func TestApplyUpdateMask_EmptyMask(t *testing.T) {
	msg := &testpb.UpdateRequest{Id: "1", Status: "active"}
	err := fieldmask.FieldMask{}.ApplyUpdateMask(msg)
	require.NoError(t, err)
	assert.Equal(t, "1", msg.GetId())
	assert.Equal(t, "active", msg.GetStatus())
}

// RFC edge case 1: Scalar field in mask, not set in request.
// update_mask: ["name", "description"], request: {name: "John"}
// Result: name = "John", description = "" (default set).
func TestApplyUpdateMask_ScalarInMaskNotSet(t *testing.T) {
	name := "John"
	msg := &testpb.UpdateRequest{
		Id:   "1",
		Name: &name,
	}

	mask := fieldmask.FromPaths("name", "description")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	assert.Equal(t, "John", msg.GetName())
	// description should be explicitly set to default ("") — meaning Has() is true.
	assert.True(t, msg.ProtoReflect().Has(
		msg.ProtoReflect().Descriptor().Fields().ByName("description"),
	))
	assert.Equal(t, "", msg.GetDescription())
	// Fields not in mask should be cleared.
	assert.Equal(t, "", msg.GetId())
}

// RFC edge case 2: Optional field (presence semantics) — clearing.
// update_mask: ["description"], request: {} (description not set)
// Result: prf.Set(fd, "") causes prf.Has(fd) == true.
func TestApplyUpdateMask_OptionalFieldClearing(t *testing.T) {
	msg := &testpb.UpdateRequest{Id: "1"}

	mask := fieldmask.FromPaths("description")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	descFD := msg.ProtoReflect().Descriptor().Fields().ByName("description")
	assert.True(t, msg.ProtoReflect().Has(descFD), "description should have presence after clearing")
	assert.Equal(t, "", msg.GetDescription())
}

// RFC edge case 3: Nested message not set, but sub-field in mask.
// update_mask: ["options.color"], request: {id: "1"}
// Result: empty Options{} created, color = "" set inside it, size not touched.
func TestApplyUpdateMask_NestedMessageNotSet(t *testing.T) {
	msg := &testpb.UpdateRequest{Id: "1"}

	mask := fieldmask.FromPaths("options.color")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	// Options should be created with color set to default.
	require.NotNil(t, msg.GetOptions())
	colorFD := msg.GetOptions().ProtoReflect().Descriptor().Fields().ByName("color")
	assert.True(t, msg.GetOptions().ProtoReflect().Has(colorFD))
	assert.Equal(t, "", msg.GetOptions().GetColor())
	// Size should not be touched (not in mask).
	sizeFD := msg.GetOptions().ProtoReflect().Descriptor().Fields().ByName("size")
	assert.False(t, msg.GetOptions().ProtoReflect().Has(sizeFD))
}

// Variant: REQUIRED nested message not set, with sub-field in mask → error.
// address has REQUIRED behavior, so if it's not set, accessing sub-fields still fails.
func TestApplyUpdateMask_RequiredNestedMessageNotSet(t *testing.T) {
	name := "John"
	msg := &testpb.UpdateRequest{
		Id:   "1",
		Name: &name,
	}

	mask := fieldmask.FromPaths("address.city")
	err := mask.ApplyUpdateMask(msg)
	require.Error(t, err)

	var behaviorErr *fieldmask.BehaviorViolationError
	require.ErrorAs(t, err, &behaviorErr)
	require.Len(t, behaviorErr.Violations, 1)
	assert.Equal(t, "address", behaviorErr.Violations[0].Field)
}

// RFC edge case 4: Repeated (list) field in mask, not set.
// update_mask: ["tags"], request: {}
// Result: tags stays as empty list (already default).
func TestApplyUpdateMask_RepeatedFieldInMaskNotSet(t *testing.T) {
	msg := &testpb.UpdateRequest{Id: "1"}

	mask := fieldmask.FromPaths("tags")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	assert.Empty(t, msg.GetTags())
}

// RFC edge case 5: Map field in mask, not set.
// update_mask: ["metadata"], request: {}
// Result: metadata stays as empty map (already default).
func TestApplyUpdateMask_MapFieldInMaskNotSet(t *testing.T) {
	msg := &testpb.UpdateRequest{Id: "1"}

	mask := fieldmask.FromPaths("metadata")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	assert.Empty(t, msg.GetMetadata())
}

// RFC edge case 6: Field is set AND in mask.
// update_mask: ["name"], request: {name: "John"}
// Result: name = "John" — kept as-is.
func TestApplyUpdateMask_FieldSetAndInMask(t *testing.T) {
	name := "John"
	msg := &testpb.UpdateRequest{
		Id:   "1",
		Name: &name,
	}

	mask := fieldmask.FromPaths("name")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	assert.Equal(t, "John", msg.GetName())
	// id not in mask — should be cleared.
	assert.Equal(t, "", msg.GetId())
}

// RFC edge case 7: REQUIRED field in mask but NOT set (BLOCKED).
// update_mask: ["name", "description"], request: {description: "new desc"}
// name has REQUIRED behavior but is not set → error.
func TestApplyUpdateMask_RequiredFieldNotSet(t *testing.T) {
	desc := "new desc"
	msg := &testpb.UpdateRequest{
		Id:          "1",
		Description: &desc,
	}

	mask := fieldmask.FromPaths("name", "description")
	err := mask.ApplyUpdateMask(msg)
	require.Error(t, err)

	var behaviorErr *fieldmask.BehaviorViolationError
	require.ErrorAs(t, err, &behaviorErr)
	require.Len(t, behaviorErr.Violations, 1)
	assert.Equal(t, "name", behaviorErr.Violations[0].Field)
	assert.Contains(t, behaviorErr.Violations[0].Description, "required")

	// Message should NOT be modified on error (fail-fast).
	assert.Equal(t, "1", msg.GetId(), "message should not be modified on validation error")
}

// RFC edge case 8: REQUIRED field NOT in mask (OK).
// update_mask: ["description"], request: {description: "new desc"}
// name has REQUIRED behavior but is NOT in mask — not affected.
func TestApplyUpdateMask_RequiredFieldNotInMask(t *testing.T) {
	desc := "new desc"
	msg := &testpb.UpdateRequest{
		Id:          "1",
		Description: &desc,
	}

	mask := fieldmask.FromPaths("description")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	assert.Equal(t, "new desc", msg.GetDescription())
}

// RFC edge case 9: REQUIRED field in mask AND set (OK).
// update_mask: ["name", "description"], request: {name: "New Name", description: "new desc"}
func TestApplyUpdateMask_RequiredFieldSetInMask(t *testing.T) {
	name := "New Name"
	desc := "new desc"
	msg := &testpb.UpdateRequest{
		Id:          "1",
		Name:        &name,
		Description: &desc,
	}

	mask := fieldmask.FromPaths("name", "description")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	assert.Equal(t, "New Name", msg.GetName())
	assert.Equal(t, "new desc", msg.GetDescription())
}

// RFC edge case 10: IMMUTABLE field in mask (BLOCKED).
// update_mask: ["code"], request: {code: "new_code"}
func TestApplyUpdateMask_ImmutableField(t *testing.T) {
	code := "new_code"
	msg := &testpb.UpdateRequest{
		Id:   "1",
		Code: &code,
	}

	mask := fieldmask.FromPaths("code")
	err := mask.ApplyUpdateMask(msg)
	require.Error(t, err)

	var behaviorErr *fieldmask.BehaviorViolationError
	require.ErrorAs(t, err, &behaviorErr)
	require.Len(t, behaviorErr.Violations, 1)
	assert.Equal(t, "code", behaviorErr.Violations[0].Field)
	assert.Contains(t, behaviorErr.Violations[0].Description, "immutable")
}

// RFC edge case 11: OUTPUT_ONLY field in mask (silently ignored).
// update_mask: ["name", "created_at"], request: {name: "John", created_at: "2026-01-01"}
func TestApplyUpdateMask_OutputOnlyField(t *testing.T) {
	name := "John"
	ts := "2026-01-01"
	msg := &testpb.UpdateRequest{
		Id:        "1",
		Name:      &name,
		CreatedAt: &ts,
	}

	mask := fieldmask.FromPaths("name", "created_at")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	assert.Equal(t, "John", msg.GetName())
	// created_at should be cleared (removed from mask, then cleared by filter).
	assert.Equal(t, "", msg.GetCreatedAt())
}

// RFC edge case 12: Nested REQUIRED field.
// update_mask: ["address.city", "address.street"], request: {address: {street: "Main St"}}
// address.city is REQUIRED but not set → error.
func TestApplyUpdateMask_NestedRequiredField(t *testing.T) {
	street := "Main St"
	msg := &testpb.UpdateRequest{
		Id: "1",
		Address: &testpb.Address{
			Street: &street,
		},
	}

	mask := fieldmask.FromPaths("address.city", "address.street")
	err := mask.ApplyUpdateMask(msg)
	require.Error(t, err)

	var behaviorErr *fieldmask.BehaviorViolationError
	require.ErrorAs(t, err, &behaviorErr)
	require.Len(t, behaviorErr.Violations, 1)
	assert.Equal(t, "address.city", behaviorErr.Violations[0].Field)
	assert.Contains(t, behaviorErr.Violations[0].Description, "required")
}

func TestApplyUpdateMask_NestedRequiredFieldSet(t *testing.T) {
	city := "Moscow"
	street := "Main St"
	msg := &testpb.UpdateRequest{
		Id: "1",
		Address: &testpb.Address{
			City:   &city,
			Street: &street,
		},
	}

	mask := fieldmask.FromPaths("address.city", "address.street")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	assert.Equal(t, "Moscow", msg.GetAddress().GetCity())
	assert.Equal(t, "Main St", msg.GetAddress().GetStreet())
}

func TestApplyUpdateMask_MultipleViolations(t *testing.T) {
	code := "abc"
	msg := &testpb.UpdateRequest{
		Id:   "1",
		Code: &code,
	}

	// name is REQUIRED (not set), code is IMMUTABLE
	mask := fieldmask.FromPaths("name", "code")
	err := mask.ApplyUpdateMask(msg)
	require.Error(t, err)

	var behaviorErr *fieldmask.BehaviorViolationError
	require.ErrorAs(t, err, &behaviorErr)
	assert.GreaterOrEqual(t, len(behaviorErr.Violations), 2)
}

func TestApplyUpdateMask_UnionWithProtectedField(t *testing.T) {
	name := "John"
	msg := &testpb.UpdateRequest{
		Id:   "1",
		Name: &name,
	}

	// Simulate handler pattern: fm.Union(FromPaths("id")).ApplyUpdateMask(in)
	mask := fieldmask.FromPaths("name").Union(fieldmask.FromPaths("id"))
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	assert.Equal(t, "1", msg.GetId())
	assert.Equal(t, "John", msg.GetName())
}

func TestApplyUpdateMask_SetDefaultForScalarNoPresence(t *testing.T) {
	msg := &testpb.UpdateRequest{
		Id:     "1",
		Status: "active",
	}

	// status is proto3 scalar (no presence) — if in mask but not "set",
	// default is already "". The field will be cleared by filter then set to default.
	mask := fieldmask.FromPaths("status")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	// status was "active" and IS in the mask, so it should be kept.
	assert.Equal(t, "active", msg.GetStatus())
}

func TestApplyUpdateMask_NestedOptionsClearing(t *testing.T) {
	color := "red"
	msg := &testpb.UpdateRequest{
		Id: "1",
		Options: &testpb.Options{
			Color: &color,
		},
	}

	// Only color in mask; size not set → should get default.
	mask := fieldmask.FromPaths("options.color", "options.size")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	require.NotNil(t, msg.GetOptions())
	assert.Equal(t, "red", msg.GetOptions().GetColor())

	sizeFD := msg.GetOptions().ProtoReflect().Descriptor().Fields().ByName("size")
	assert.True(t, msg.GetOptions().ProtoReflect().Has(sizeFD))
	assert.Equal(t, int32(0), msg.GetOptions().GetSize())
}

func TestApplyUpdateMask_PriorityDefault(t *testing.T) {
	msg := &testpb.UpdateRequest{Id: "1"}

	mask := fieldmask.FromPaths("priority")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	prioFD := msg.ProtoReflect().Descriptor().Fields().ByName("priority")
	assert.True(t, msg.ProtoReflect().Has(prioFD))
	assert.Equal(t, int32(0), msg.GetPriority())
}

func TestApplyUpdateMask_ImmutableFieldNotSet(t *testing.T) {
	// IMMUTABLE field in mask but not set → should also be an error.
	msg := &testpb.UpdateRequest{Id: "1"}

	mask := fieldmask.FromPaths("code")
	err := mask.ApplyUpdateMask(msg)
	require.Error(t, err)

	var behaviorErr *fieldmask.BehaviorViolationError
	require.ErrorAs(t, err, &behaviorErr)
	require.Len(t, behaviorErr.Violations, 1)
	assert.Contains(t, behaviorErr.Violations[0].Description, "immutable")
}

func TestApplyUpdateMask_OutputOnlyRemovedFromMask(t *testing.T) {
	ts := "2026-01-01"
	msg := &testpb.UpdateRequest{
		CreatedAt: &ts,
	}

	mask := fieldmask.FromPaths("created_at")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	// After apply, created_at should not be present (it was the only field in mask,
	// got removed, so effectively empty mask → no fields kept).
	assert.Equal(t, "", msg.GetCreatedAt())
}

func TestApplyUpdateMask_MessageNotMutatedOnError(t *testing.T) {
	name := "Original"
	desc := "Original desc"
	msg := &testpb.UpdateRequest{
		Id:          "1",
		Name:        &name,
		Description: &desc,
	}

	original := proto.Clone(msg).(*testpb.UpdateRequest)

	// code is IMMUTABLE → error
	mask := fieldmask.FromPaths("name", "description", "code")
	err := mask.ApplyUpdateMask(msg)
	require.Error(t, err)

	// Message should be unchanged.
	assert.True(t, proto.Equal(original, msg), "message should not be modified on validation error")
}

func TestApplyUpdateMask_FieldNotInSchema(t *testing.T) {
	name := "John"
	msg := &testpb.UpdateRequest{
		Id:   "1",
		Name: &name,
	}

	// "nonexistent" field should be silently ignored (no field descriptor found).
	mask := fieldmask.FromPaths("name", "nonexistent")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	assert.Equal(t, "John", msg.GetName())
}

// AIP-203 IDENTIFIER: field in mask is rejected like IMMUTABLE.
// update_mask: ["resource_name"], request: {resource_name: "items/123"}
func TestApplyUpdateMask_IdentifierFieldInMask(t *testing.T) {
	resourceName := "items/123"
	msg := &testpb.UpdateRequest{
		Id:           "1",
		ResourceName: &resourceName,
	}

	mask := fieldmask.FromPaths("resource_name")
	err := mask.ApplyUpdateMask(msg)
	require.Error(t, err)

	var behaviorErr *fieldmask.BehaviorViolationError
	require.ErrorAs(t, err, &behaviorErr)
	require.Len(t, behaviorErr.Violations, 1)
	assert.Equal(t, "resource_name", behaviorErr.Violations[0].Field)
	assert.Contains(t, behaviorErr.Violations[0].Description, "identifier")
}

// AIP-203 IDENTIFIER not in mask: identifier is set to route the update,
// other fields are updated, identifier is preserved.
func TestApplyUpdateMask_IdentifierNotInMask(t *testing.T) {
	resourceName := "items/123"
	name := "John"
	msg := &testpb.UpdateRequest{
		Id:           "1",
		ResourceName: &resourceName,
		Name:         &name,
	}

	mask := fieldmask.FromPaths("name")
	err := mask.ApplyUpdateMask(msg)
	require.NoError(t, err)

	assert.Equal(t, "items/123", msg.GetResourceName(), "identifier preserved when not in mask")
	assert.Equal(t, "John", msg.GetName())
}

// IDENTIFIER in mask but not set in the request still violates: matching
// IMMUTABLE semantics so callers can't smuggle identifier mutation through
// an empty value.
func TestApplyUpdateMask_IdentifierFieldInMaskNotSet(t *testing.T) {
	msg := &testpb.UpdateRequest{Id: "1"}

	mask := fieldmask.FromPaths("resource_name")
	err := mask.ApplyUpdateMask(msg)
	require.Error(t, err)

	var behaviorErr *fieldmask.BehaviorViolationError
	require.ErrorAs(t, err, &behaviorErr)
	require.Len(t, behaviorErr.Violations, 1)
	assert.Equal(t, "resource_name", behaviorErr.Violations[0].Field)
	assert.Contains(t, behaviorErr.Violations[0].Description, "identifier")
}
