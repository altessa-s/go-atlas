// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/behavior"
)

type baseEntity struct {
	ID         string    `behavior:"identifier"`
	CreateTime time.Time `behavior:"output_only"`
}

func TestStripPromotesEmbeddedValueStruct(t *testing.T) {
	t.Parallel()

	type bucket struct {
		baseEntity // embedded by value — fields promote to the parent
		Name       string
	}

	b := bucket{
		baseEntity: baseEntity{ID: "id-1", CreateTime: time.Unix(1, 0).UTC()},
		Name:       "photos",
	}
	require.NoError(t, behavior.StripCreate(&b))

	require.Empty(t, b.ID, "promoted identifier cleared")
	require.True(t, b.CreateTime.IsZero(), "promoted output_only cleared")
	require.Equal(t, "photos", b.Name, "outer field kept")
}

func TestStripPromotesEmbeddedPointerStruct(t *testing.T) {
	t.Parallel()

	type bucket struct {
		*baseEntity // embedded by pointer — fields still promote to the parent
		Name        string
	}

	b := bucket{
		baseEntity: &baseEntity{ID: "id-1", CreateTime: time.Unix(1, 0).UTC()},
		Name:       "photos",
	}
	require.NoError(t, behavior.StripCreate(&b))

	require.Empty(t, b.ID, "promoted-via-pointer identifier cleared")
	require.True(t, b.CreateTime.IsZero(), "promoted-via-pointer output_only cleared")
	require.Equal(t, "photos", b.Name, "outer field kept")
}

func TestStripNilEmbeddedPointerIsNoOp(t *testing.T) {
	t.Parallel()

	type bucket struct {
		*baseEntity // nil at strip time → promoted fields unreachable, skipped
		Name        string
	}

	b := bucket{Name: "photos"}
	require.NoError(t, behavior.StripCreate(&b), "nil embedded pointer must not panic")
	require.Nil(t, b.baseEntity)
	require.Equal(t, "photos", b.Name)
}

func TestStripStrictUsesPromotedPaths(t *testing.T) {
	t.Parallel()

	type bucket struct {
		baseEntity
		Name string
	}

	b := bucket{baseEntity: baseEntity{ID: "id-1"}}
	err := behavior.StripCreate(&b, behavior.WithStrict())

	var ve *behavior.ViolationError
	require.ErrorAs(t, err, &ve)
	require.Len(t, ve.Violations, 1)
	require.Equal(t, "ID", ve.Violations[0].Path, "embedded field path is promoted, not baseEntity.ID")
}

func TestStripMaxDepthClampedForFlatStruct(t *testing.T) {
	t.Parallel()

	type flat struct {
		ID string `behavior:"identifier"`
	}

	// A non-positive maxDepth must not reject a flat struct; it is clamped to 1.
	for _, depth := range []int{0, -5} {
		v := &flat{ID: "x"}
		require.NoError(t, behavior.StripCreate(v, behavior.WithMaxDepth(depth)))
		require.Empty(t, v.ID)
	}
}

func TestStripInterfaceFieldIsLeaf(t *testing.T) {
	t.Parallel()

	type inner struct {
		Secret string `behavior:"input_only"`
	}
	type holder struct {
		Cleared any `behavior:"input_only"` // tagged interface field is cleared whole
		Opaque  any // untagged: the dynamic value is not inspected
	}

	h := &holder{
		Cleared: "secret",
		Opaque:  inner{Secret: "kept"},
	}
	require.NoError(t, behavior.StripResponse(h))

	require.Nil(t, h.Cleared, "tagged interface field cleared")
	got, ok := h.Opaque.(inner)
	require.True(t, ok)
	require.Equal(t, "kept", got.Secret, "behavior tag inside an interface value is not applied")
}
