// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/altessa-s/go-atlas/domain/proto/internal/behavior"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"

	testpb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func TestSubtreeHasAny_DirectAnnotation(t *testing.T) {
	t.Parallel()

	md := (&testpb.Profile{}).ProtoReflect().Descriptor()

	assert.True(t, behavior.SubtreeHasAny(md, annotations.FieldBehavior_INPUT_ONLY))
	assert.True(t, behavior.SubtreeHasAny(md, annotations.FieldBehavior_OUTPUT_ONLY))
	assert.False(t, behavior.SubtreeHasAny(md, annotations.FieldBehavior_NON_EMPTY_DEFAULT))
}

func TestSubtreeHasAny_TransitiveThroughNested(t *testing.T) {
	t.Parallel()

	// NestedGetResourceRequest has only a REQUIRED annotation of its own, but
	// no OUTPUT_ONLY anywhere in its tree (Options wraps a plain FieldMask).
	md := (&testpb.NestedGetResourceRequest{}).ProtoReflect().Descriptor()

	assert.True(t, behavior.SubtreeHasAny(md, annotations.FieldBehavior_REQUIRED))
	assert.False(t, behavior.SubtreeHasAny(md, annotations.FieldBehavior_OUTPUT_ONLY))
}

func TestSubtreeHasAny_UnannotatedType(t *testing.T) {
	t.Parallel()

	md := (&fieldmaskpb.FieldMask{}).ProtoReflect().Descriptor()

	assert.False(t, behavior.SubtreeHasAny(md,
		annotations.FieldBehavior_INPUT_ONLY,
		annotations.FieldBehavior_OUTPUT_ONLY,
		annotations.FieldBehavior_IDENTIFIER,
	))
}

func TestSubtreeHasAny_EmptySet(t *testing.T) {
	t.Parallel()

	md := (&testpb.Profile{}).ProtoReflect().Descriptor()

	assert.False(t, behavior.SubtreeHasAny(md))
}

// TestSubtreeHasAny_RecursiveType pins termination on self-referential
// message types (structpb.Value ↔ Struct ↔ ListValue).
func TestSubtreeHasAny_RecursiveType(t *testing.T) {
	t.Parallel()

	md := (&structpb.Struct{}).ProtoReflect().Descriptor()

	assert.False(t, behavior.SubtreeHasAny(md, annotations.FieldBehavior_INPUT_ONLY))
}
