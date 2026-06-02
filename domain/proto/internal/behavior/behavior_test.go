// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/proto/internal/behavior"

	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/reflect/protoreflect"

	testpb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func fieldByName(t *testing.T, msg protoreflect.Message, name string) protoreflect.FieldDescriptor {
	t.Helper()
	fd := msg.Descriptor().Fields().ByName(protoreflect.Name(name))
	require.NotNilf(t, fd, "field %q not found", name)
	return fd
}

func TestGet(t *testing.T) {
	t.Parallel()

	msg := (&testpb.Resource{}).ProtoReflect()

	cases := []struct {
		field string
		want  []annotations.FieldBehavior
	}{
		{"description", nil},
		{"profile", nil},
		{"id", []annotations.FieldBehavior{annotations.FieldBehavior_IDENTIFIER}},
		{"name", []annotations.FieldBehavior{annotations.FieldBehavior_REQUIRED}},
		{"tenant_id", []annotations.FieldBehavior{annotations.FieldBehavior_IMMUTABLE}},
		{"create_time", []annotations.FieldBehavior{annotations.FieldBehavior_OUTPUT_ONLY}},
		{"password", []annotations.FieldBehavior{annotations.FieldBehavior_INPUT_ONLY}},
		{"slug", []annotations.FieldBehavior{
			annotations.FieldBehavior_REQUIRED,
			annotations.FieldBehavior_IMMUTABLE,
		}},
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			t.Parallel()
			got := behavior.Get(fieldByName(t, msg, tc.field))
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestHas(t *testing.T) {
	t.Parallel()

	msg := (&testpb.Resource{}).ProtoReflect()
	slug := fieldByName(t, msg, "slug")
	plain := fieldByName(t, msg, "description")

	assert.True(t, behavior.Has(slug, annotations.FieldBehavior_REQUIRED))
	assert.True(t, behavior.Has(slug, annotations.FieldBehavior_IMMUTABLE))
	assert.False(t, behavior.Has(slug, annotations.FieldBehavior_OUTPUT_ONLY))
	assert.False(t, behavior.Has(plain, annotations.FieldBehavior_REQUIRED))
}

func TestHasAny(t *testing.T) {
	t.Parallel()

	msg := (&testpb.Resource{}).ProtoReflect()
	slug := fieldByName(t, msg, "slug")
	password := fieldByName(t, msg, "password")
	plain := fieldByName(t, msg, "description")

	// Empty set always returns false.
	assert.False(t, behavior.HasAny(slug))

	// Match on the first set member.
	assert.True(t, behavior.HasAny(slug,
		annotations.FieldBehavior_REQUIRED,
		annotations.FieldBehavior_OUTPUT_ONLY,
	))

	// Match on the second set member.
	assert.True(t, behavior.HasAny(slug,
		annotations.FieldBehavior_OUTPUT_ONLY,
		annotations.FieldBehavior_IMMUTABLE,
	))

	// INPUT_ONLY does not intersect with the write-side behaviors.
	assert.False(t, behavior.HasAny(password,
		annotations.FieldBehavior_OUTPUT_ONLY,
		annotations.FieldBehavior_IMMUTABLE,
	))
	assert.True(t, behavior.HasAny(password, annotations.FieldBehavior_INPUT_ONLY))

	// Field without any annotation never matches.
	assert.False(t, behavior.HasAny(plain,
		annotations.FieldBehavior_REQUIRED,
		annotations.FieldBehavior_OUTPUT_ONLY,
		annotations.FieldBehavior_IMMUTABLE,
		annotations.FieldBehavior_IDENTIFIER,
		annotations.FieldBehavior_INPUT_ONLY,
	))
}
