// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package behavior_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/core/types/optional"
	"github.com/altessa-s/go-atlas/domain/behavior"
)

// resource exercises every representation: plain, pointer, Optional, and an
// opaque struct (time.Time) that must be treated as a scalar leaf.
type resource struct {
	ID         string                    `behavior:"identifier"`
	TenantID   string                    `behavior:"immutable"`
	CreateTime time.Time                 `behavior:"output_only"`
	Password   *string                   `behavior:"input_only"`
	Name       string                    `behavior:"required"`
	Policy     optional.Optional[string] `behavior:"output_only"`
	Quota      *int                      `behavior:"output_only"`
	Plain      string
}

func newResource() *resource {
	pw := "secret"
	q := 42
	return &resource{
		ID:         "id-1",
		TenantID:   "tenant-1",
		CreateTime: time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC),
		Password:   &pw,
		Name:       "bucket",
		Policy:     optional.Some("policy"),
		Quota:      &q,
		Plain:      "keep",
	}
}

func TestStripCreate(t *testing.T) {
	t.Parallel()

	r := newResource()
	require.NoError(t, behavior.StripCreate(r))

	require.Empty(t, r.ID, "identifier cleared")
	require.True(t, r.CreateTime.IsZero(), "output_only cleared")
	require.Nil(t, r.Quota, "output_only *T cleared to nil")
	require.True(t, r.Policy.IsNone(), "output_only Optional cleared to None")

	require.Equal(t, "tenant-1", r.TenantID, "immutable kept on create")
	require.Equal(t, "bucket", r.Name, "required kept")
	require.NotNil(t, r.Password, "input_only kept on create")
	require.Equal(t, "keep", r.Plain, "untagged kept")
}

func TestStripUpdate(t *testing.T) {
	t.Parallel()

	r := newResource()
	require.NoError(t, behavior.StripUpdate(r))

	require.Empty(t, r.ID)
	require.Empty(t, r.TenantID, "immutable cleared on update")
	require.True(t, r.CreateTime.IsZero())
	require.Nil(t, r.Quota)
	require.True(t, r.Policy.IsNone())

	require.Equal(t, "bucket", r.Name, "required kept")
	require.NotNil(t, r.Password, "input_only kept on update")
}

func TestStripResponse(t *testing.T) {
	t.Parallel()

	r := newResource()
	require.NoError(t, behavior.StripResponse(r))

	require.Nil(t, r.Password, "input_only cleared")
	require.Equal(t, "id-1", r.ID, "identifier kept in response")
	require.Equal(t, "tenant-1", r.TenantID)
	require.False(t, r.CreateTime.IsZero(), "output_only kept in response")
	require.True(t, r.Policy.IsSome())
}

func TestStripNoBehaviorsIsNoOp(t *testing.T) {
	t.Parallel()

	r := newResource()
	require.NoError(t, behavior.Strip(r))
	require.Equal(t, newResource(), r)
}

func TestStripMultiBehaviorMatchesEither(t *testing.T) {
	t.Parallel()

	type m struct {
		Field string `behavior:"required,immutable"`
	}

	v := &m{Field: "x"}
	require.NoError(t, behavior.StripUpdate(v)) // update set contains immutable
	require.Empty(t, v.Field)

	v = &m{Field: "x"}
	require.NoError(t, behavior.StripCreate(v)) // create set lacks immutable/required
	require.Equal(t, "x", v.Field)
}

func TestStripStrictReportsViolationsWithoutMutating(t *testing.T) {
	t.Parallel()

	r := newResource()
	err := behavior.StripCreate(r, behavior.WithStrict())

	var ve *behavior.ViolationError
	require.ErrorAs(t, err, &ve)

	paths := make([]string, 0, len(ve.Violations))
	for _, v := range ve.Violations {
		paths = append(paths, v.Path)
	}
	require.ElementsMatch(t, []string{"ID", "CreateTime", "Policy", "Quota"}, paths)

	require.Equal(t, newResource(), r, "strict mode leaves the struct untouched")
}

func TestStripStrictSkipsEmptyFields(t *testing.T) {
	t.Parallel()

	// Only ID is populated; the other output_only/identifier fields are zero
	// and must not be reported.
	r := &resource{ID: "id-1"}
	err := behavior.StripCreate(r, behavior.WithStrict())

	var ve *behavior.ViolationError
	require.ErrorAs(t, err, &ve)
	require.Len(t, ve.Violations, 1)
	require.Equal(t, "ID", ve.Violations[0].Path)
	require.Equal(t, behavior.Identifier, ve.Violations[0].Kind)
}

func TestStripNested(t *testing.T) {
	t.Parallel()

	type inner struct {
		Secret string `behavior:"input_only"`
		Keep   string
	}
	type outer struct {
		Ptr   *inner
		Val   inner
		List  []inner
		Ptrs  []*inner
		ByKey map[string]inner
	}

	o := &outer{
		Ptr:   &inner{Secret: "s", Keep: "k"},
		Val:   inner{Secret: "s", Keep: "k"},
		List:  []inner{{Secret: "s", Keep: "k"}},
		Ptrs:  []*inner{{Secret: "s", Keep: "k"}},
		ByKey: map[string]inner{"a": {Secret: "s", Keep: "k"}},
	}

	require.NoError(t, behavior.StripResponse(o))

	require.Empty(t, o.Ptr.Secret)
	require.Empty(t, o.Val.Secret)
	require.Empty(t, o.List[0].Secret)
	require.Empty(t, o.Ptrs[0].Secret)
	require.Empty(t, o.ByKey["a"].Secret)

	require.Equal(t, "k", o.Ptr.Keep)
	require.Equal(t, "k", o.Val.Keep)
	require.Equal(t, "k", o.List[0].Keep)
	require.Equal(t, "k", o.Ptrs[0].Keep)
	require.Equal(t, "k", o.ByKey["a"].Keep, "non-tagged map-value field is preserved")
}

func TestStripArrayOfStructs(t *testing.T) {
	t.Parallel()

	type inner struct {
		Secret string `behavior:"input_only"`
		Keep   string
	}
	type holder struct {
		Items [2]inner
	}

	h := &holder{Items: [2]inner{{Secret: "s", Keep: "k"}, {Secret: "s", Keep: "k"}}}
	require.NoError(t, behavior.StripResponse(h))

	for i := range h.Items {
		require.Empty(t, h.Items[i].Secret, "tagged array element field cleared")
		require.Equal(t, "k", h.Items[i].Keep, "untagged array element field preserved")
	}
}

func TestStripNestedCollections(t *testing.T) {
	t.Parallel()

	type inner struct {
		Secret string `behavior:"input_only"`
		Keep   string
	}
	type holder struct {
		Grid        [][]inner
		ByKeyList   map[string][]inner
		ByKeyArray  map[string][2]inner
		ByKeyByKey  map[string]map[string]inner
		ListOfByKey []map[string]inner
		PtrChain    map[string][]*inner
	}

	mk := func() inner { return inner{Secret: "s", Keep: "k"} }
	h := &holder{
		Grid:        [][]inner{{mk()}, {mk(), mk()}},
		ByKeyList:   map[string][]inner{"a": {mk()}},
		ByKeyArray:  map[string][2]inner{"a": {mk(), mk()}},
		ByKeyByKey:  map[string]map[string]inner{"a": {"b": mk()}},
		ListOfByKey: []map[string]inner{{"a": mk()}},
		PtrChain:    map[string][]*inner{"a": {{Secret: "s", Keep: "k"}, nil}},
	}

	require.NoError(t, behavior.StripResponse(h))

	for _, got := range []inner{
		h.Grid[1][1],
		h.ByKeyList["a"][0],
		h.ByKeyArray["a"][1],
		h.ByKeyByKey["a"]["b"],
		h.ListOfByKey[0]["a"],
		*h.PtrChain["a"][0],
	} {
		require.Empty(t, got.Secret, "tagged field inside a nested collection cleared")
		require.Equal(t, "k", got.Keep, "untagged field inside a nested collection preserved")
	}
}

func TestStripStrictReportsNestedCollectionPaths(t *testing.T) {
	t.Parallel()

	type inner struct {
		Secret string `behavior:"input_only"`
	}
	type holder struct {
		Grid  [][]inner
		ByKey map[string][]inner
	}

	h := &holder{
		Grid:  [][]inner{{{}}, {{}, {Secret: "s"}}},
		ByKey: map[string][]inner{"a": {{Secret: "s"}}},
	}

	err := behavior.StripResponse(h, behavior.WithStrict())

	var ve *behavior.ViolationError
	require.ErrorAs(t, err, &ve)

	paths := make([]string, 0, len(ve.Violations))
	for _, v := range ve.Violations {
		paths = append(paths, v.Path)
	}
	require.ElementsMatch(t, []string{`Grid[1][1].Secret`, `ByKey["a"][0].Secret`}, paths)
}

func TestStripStrictReportsSliceAndMapPaths(t *testing.T) {
	t.Parallel()

	type inner struct {
		Secret string `behavior:"input_only"`
	}
	type holder struct {
		List  []inner
		ByKey map[string]inner
	}

	h := &holder{
		List:  []inner{{}, {Secret: "s"}},
		ByKey: map[string]inner{"a": {Secret: "s"}},
	}

	err := behavior.StripResponse(h, behavior.WithStrict())

	var ve *behavior.ViolationError
	require.ErrorAs(t, err, &ve)

	paths := make([]string, 0, len(ve.Violations))
	for _, v := range ve.Violations {
		paths = append(paths, v.Path)
	}
	require.ElementsMatch(t, []string{`List[1].Secret`, `ByKey["a"].Secret`}, paths)

	require.Equal(t, "s", h.ByKey["a"].Secret, "strict mode leaves map values untouched")
}

func TestStripEmptyWithKindsResetsDefaults(t *testing.T) {
	t.Parallel()

	r := newResource()
	require.NoError(t, behavior.StripUpdate(r, behavior.WithKinds()))
	require.Equal(t, newResource(), r, "empty WithKinds clears the default set, making Strip a no-op")
}

func TestStripCustomTagName(t *testing.T) {
	t.Parallel()

	type m struct {
		Secret string `policy:"input_only"`
		Other  string `behavior:"input_only"`
	}

	v := &m{Secret: "s", Other: "o"}
	require.NoError(t, behavior.StripResponse(v, behavior.WithTagName("policy")))
	require.Empty(t, v.Secret, "custom tag honored")
	require.Equal(t, "o", v.Other, "default tag ignored under custom tag name")
}

func TestStripMaxDepthExceeded(t *testing.T) {
	t.Parallel()

	type lvl struct {
		Name string `behavior:"output_only"`
		Next *lvl
	}

	root := &lvl{Name: "0", Next: &lvl{Name: "1", Next: &lvl{Name: "2"}}}
	err := behavior.StripCreate(root, behavior.WithMaxDepth(1))
	require.ErrorIs(t, err, behavior.ErrMaxDepthExceeded)
}

func TestStripUnknownBehaviorToken(t *testing.T) {
	t.Parallel()

	type m struct {
		Field string `behavior:"bogus"`
	}

	err := behavior.StripCreate(&m{Field: "x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown behavior")
}

func TestStripInvalidInput(t *testing.T) {
	t.Parallel()

	var p *resource
	require.NoError(t, behavior.StripCreate(p), "typed nil pointer is a no-op")

	n := 5
	require.Error(t, behavior.StripCreate(&n), "pointer to non-struct is rejected")
}

func TestStripDoesNotDescendIntoOptionalStruct(t *testing.T) {
	t.Parallel()

	// A tagged field inside an Optional-wrapped struct must NOT be reached:
	// Optional is an opaque leaf, cleared whole only when itself tagged.
	type inner struct {
		Secret string `behavior:"input_only"`
	}
	type m struct {
		Wrapped optional.Optional[inner]
	}

	v := &m{Wrapped: optional.Some(inner{Secret: "s"})}
	require.NoError(t, behavior.StripResponse(v))

	got, ok := v.Wrapped.Get()
	require.True(t, ok, "untagged Optional is left intact")
	require.Equal(t, "s", got.Secret, "behavior tag inside an Optional is not applied")
}
