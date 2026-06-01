// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package fieldbehavior_test

import (
	"errors"
	"slices"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/domain/proto/fieldbehavior"

	"google.golang.org/genproto/googleapis/api/annotations"

	testpb "github.com/altessa-s/go-atlas/proto/gen/fieldbehaviortest/v1"
)

func fullResource() *testpb.Resource {
	return &testpb.Resource{
		Id:          "ID-001",
		Name:        "n",
		TenantId:    "T",
		CreateTime:  "2026-01-01",
		Password:    "secret",
		Description: "plain",
		Profile: &testpb.Profile{
			Id:          "P-1",
			DisplayName: "dn",
			UpdatedAt:   "2026-01-02",
			Secret:      "ps",
		},
		Audit: &testpb.Profile{
			Id:          "P-A",
			DisplayName: "audit",
		},
		Aliases: []*testpb.Profile{
			{Id: "A-1", DisplayName: "a1", UpdatedAt: "u1", Secret: "s1"},
			{Id: "A-2", DisplayName: "a2", Secret: "s2"},
		},
		Labels: map[string]*testpb.Profile{
			"k1": {Id: "L-1", DisplayName: "l1", UpdatedAt: "u-l"},
		},
		Tags:       []string{"t1", "t2"},
		ServerMeta: map[string]string{"sm": "v"},
		Source:     &testpb.Resource_SourceUrl{SourceUrl: "https://example"},
		Slug:       "slug-1",
	}
}

func TestStripCreate_ClearsOutputOnlyAndIdentifier(t *testing.T) {
	t.Parallel()

	r := fullResource()
	require.NoError(t, fieldbehavior.StripCreate(r))

	assert.Empty(t, r.GetId(), "IDENTIFIER must be cleared")
	assert.Empty(t, r.GetCreateTime(), "OUTPUT_ONLY scalar must be cleared")
	assert.Nil(t, r.GetAudit(), "OUTPUT_ONLY nested message must be cleared")
	assert.Empty(t, r.GetTags(), "OUTPUT_ONLY list must be cleared")
	assert.Empty(t, r.GetServerMeta(), "OUTPUT_ONLY map must be cleared")

	// IMMUTABLE is NOT stripped on create.
	assert.Equal(t, "T", r.GetTenantId())
	assert.Equal(t, "slug-1", r.GetSlug())

	// REQUIRED, description, profile-without-annotation kept.
	assert.Equal(t, "n", r.GetName())
	assert.Equal(t, "plain", r.GetDescription())
	require.NotNil(t, r.GetProfile())

	// INPUT_ONLY survives on create (will be consumed by server).
	assert.Equal(t, "secret", r.GetPassword())
	assert.Equal(t, "ps", r.GetProfile().GetSecret())

	// Nested IDENTIFIER is cleared via recursion.
	assert.Empty(t, r.GetProfile().GetId())
	// Nested OUTPUT_ONLY too.
	assert.Empty(t, r.GetProfile().GetUpdatedAt())

	// Aliases are recursed: each element's IDENTIFIER + OUTPUT_ONLY cleared,
	// secret kept.
	require.Len(t, r.GetAliases(), 2)
	for _, a := range r.GetAliases() {
		assert.Empty(t, a.GetId())
		assert.Empty(t, a.GetUpdatedAt())
		assert.NotEmpty(t, a.GetDisplayName())
		assert.NotEmpty(t, a.GetSecret())
	}

	// Labels map recursed: nested IDENTIFIER and OUTPUT_ONLY cleared.
	require.Contains(t, r.GetLabels(), "k1")
	assert.Empty(t, r.GetLabels()["k1"].GetId())
	assert.Empty(t, r.GetLabels()["k1"].GetUpdatedAt())
}

func TestStripUpdate_ClearsImmutableToo(t *testing.T) {
	t.Parallel()

	r := fullResource()
	require.NoError(t, fieldbehavior.StripUpdate(r))

	assert.Empty(t, r.GetId())
	assert.Empty(t, r.GetCreateTime())
	assert.Empty(t, r.GetTenantId(), "IMMUTABLE must be cleared on update")
	assert.Empty(t, r.GetSlug(), "REQUIRED+IMMUTABLE must be cleared on update")
	assert.Equal(t, "n", r.GetName(), "REQUIRED-only kept")
	assert.Equal(t, "secret", r.GetPassword(), "INPUT_ONLY kept on update")
}

func TestStripResponse_ClearsInputOnly(t *testing.T) {
	t.Parallel()

	r := fullResource()
	require.NoError(t, fieldbehavior.StripResponse(r))

	assert.Empty(t, r.GetPassword(), "top-level INPUT_ONLY cleared")
	assert.Empty(t, r.GetProfile().GetSecret(), "nested INPUT_ONLY cleared")
	for _, a := range r.GetAliases() {
		assert.Empty(t, a.GetSecret(), "INPUT_ONLY in repeated element cleared")
	}

	// Oneof INPUT_ONLY case cleared.
	rWithToken := fullResource()
	rWithToken.Source = &testpb.Resource_SourceToken{SourceToken: "abc"}
	require.NoError(t, fieldbehavior.StripResponse(rWithToken))
	assert.Nil(t, rWithToken.GetSource(), "INPUT_ONLY oneof case cleared")

	// Oneof non-INPUT_ONLY case left alone.
	rWithURL := fullResource()
	require.NoError(t, fieldbehavior.StripResponse(rWithURL))
	assert.NotNil(t, rWithURL.GetSource())

	// Server-write fields preserved on response.
	assert.NotEmpty(t, r.GetId())
	assert.NotEmpty(t, r.GetCreateTime())
}

func TestStrip_NoBehaviorsIsNoop(t *testing.T) {
	t.Parallel()

	r := fullResource()
	require.NoError(t, fieldbehavior.Strip(r))
	assert.Equal(t, "ID-001", r.GetId(), "Strip without WithBehaviors must not mutate")
}

func TestStrip_NilMessageIsNoop(t *testing.T) {
	t.Parallel()
	require.NoError(t, fieldbehavior.Strip(nil, fieldbehavior.WithBehaviors(annotations.FieldBehavior_OUTPUT_ONLY)))
}

func TestStripCreate_Strict_ReportsViolations(t *testing.T) {
	t.Parallel()

	r := fullResource()
	err := fieldbehavior.StripCreate(r, fieldbehavior.WithStrict())
	require.Error(t, err)

	var vErr *fieldbehavior.BehaviorViolationError
	require.True(t, errors.As(err, &vErr))

	paths := make([]string, 0, len(vErr.Violations))
	for _, v := range vErr.Violations {
		paths = append(paths, v.Path)
	}
	sort.Strings(paths)

	expected := []string{
		"aliases[0].id",
		"aliases[0].updated_at",
		"aliases[1].id",
		"audit",
		"create_time",
		"id",
		`labels["k1"].id`,
		`labels["k1"].updated_at`,
		"profile.id",
		"profile.updated_at",
		"server_meta",
		"tags",
	}
	sort.Strings(expected)
	assert.Equal(t, expected, paths)

	// Strict must NOT mutate.
	assert.Equal(t, "ID-001", r.GetId())
	assert.NotNil(t, r.GetAudit())
}

func TestStrip_Strict_NoViolationsReturnsNilAndDoesNotMutate(t *testing.T) {
	t.Parallel()

	// Resource with only safe (non-stripped) fields.
	r := &testpb.Resource{Name: "n", Description: "d"}
	err := fieldbehavior.StripCreate(r, fieldbehavior.WithStrict())
	require.NoError(t, err)
	assert.Equal(t, "n", r.GetName())
	assert.Equal(t, "d", r.GetDescription())
}

func TestStrip_MultipleBehaviorsOnSameField(t *testing.T) {
	t.Parallel()

	r := fullResource()
	require.NoError(t, fieldbehavior.StripUpdate(r))
	assert.Empty(t, r.GetSlug(), "REQUIRED+IMMUTABLE field matches IMMUTABLE in update set")
}

func TestStrip_NestedOutputOnlyNotTraversed(t *testing.T) {
	t.Parallel()

	r := &testpb.Resource{
		Audit: &testpb.Profile{
			DisplayName: "should-survive-when-stripping-input-only",
			Secret:      "must-be-cleared",
		},
	}
	require.NoError(t, fieldbehavior.StripResponse(r))
	// audit has OUTPUT_ONLY at field level; StripResponse strips INPUT_ONLY,
	// not OUTPUT_ONLY — so audit subtree is traversed and its INPUT_ONLY
	// secret cleared, while display_name survives.
	require.NotNil(t, r.GetAudit())
	assert.Equal(t, "should-survive-when-stripping-input-only", r.GetAudit().GetDisplayName())
	assert.Empty(t, r.GetAudit().GetSecret())
}

func TestWithMaxDepth_ExceededReturnsErr(t *testing.T) {
	t.Parallel()

	r := fullResource()
	err := fieldbehavior.StripCreate(r, fieldbehavior.WithMaxDepth(0))
	require.Error(t, err)
	assert.True(t, errors.Is(err, fieldbehavior.ErrMaxDepthExceeded))
}

func TestStrip_CustomBehaviorSet(t *testing.T) {
	t.Parallel()

	r := fullResource()
	require.NoError(t, fieldbehavior.Strip(r, fieldbehavior.WithBehaviors(annotations.FieldBehavior_REQUIRED)))
	assert.Empty(t, r.GetName(), "REQUIRED set was passed, name must be cleared")
	assert.Empty(t, r.GetSlug(), "slug is REQUIRED+IMMUTABLE, matches REQUIRED in custom set")
	assert.Equal(t, "plain", r.GetDescription())
}

func TestStrip_OnEmptyResourceNoop(t *testing.T) {
	t.Parallel()

	r := &testpb.Resource{}
	require.NoError(t, fieldbehavior.StripCreate(r))
	require.NoError(t, fieldbehavior.StripUpdate(r))
	require.NoError(t, fieldbehavior.StripResponse(r))
}

func TestBehaviorViolationError_Message(t *testing.T) {
	t.Parallel()

	err := &fieldbehavior.BehaviorViolationError{
		Violations: []fieldbehavior.BehaviorViolation{
			{Path: "id", Behavior: annotations.FieldBehavior_IDENTIFIER},
			{Path: "create_time", Behavior: annotations.FieldBehavior_OUTPUT_ONLY},
		},
	}
	msg := err.Error()
	assert.Contains(t, msg, "2 violation(s)")
	assert.Contains(t, msg, "id (IDENTIFIER)")
	assert.Contains(t, msg, "create_time (OUTPUT_ONLY)")
}

func TestStrip_PreservesNonStrippedFieldsAcrossWalk(t *testing.T) {
	t.Parallel()

	r := fullResource()
	require.NoError(t, fieldbehavior.StripCreate(r))

	assert.Equal(t, "plain", r.GetDescription())
	assert.Equal(t, "n", r.GetName())
	assert.Equal(t, "slug-1", r.GetSlug())
	assert.Equal(t, "dn", r.GetProfile().GetDisplayName())

	require.Len(t, r.GetAliases(), 2)
	names := []string{r.GetAliases()[0].GetDisplayName(), r.GetAliases()[1].GetDisplayName()}
	assert.True(t, slices.Equal(names, []string{"a1", "a2"}))
}
