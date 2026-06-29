// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package reasoncode_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/grpc/interceptors/protovalidator/reasoncode"
)

// fakeViolation is a test [reasoncode.Violation] with explicitly set context.
type fakeViolation struct {
	ruleID    string
	fieldName string
	value     float64
	hasValue  bool
	lower     float64
	inclusive bool
	hasLower  bool
}

func (f fakeViolation) RuleID() string                    { return f.ruleID }
func (f fakeViolation) FieldName() string                 { return f.fieldName }
func (f fakeViolation) NumericValue() (float64, bool)     { return f.value, f.hasValue }
func (f fakeViolation) LowerBound() (float64, bool, bool) { return f.lower, f.inclusive, f.hasLower }

func TestResolver_Resolve_StandardRules(t *testing.T) {
	r := reasoncode.NewResolver(nil)

	tests := []struct {
		ruleID string
		want   string
	}{
		{"int64.gte", reasoncode.InvalidMinLengthOrValue},
		{"uint32.gte", reasoncode.InvalidMinLengthOrValue},
		{"double.gt", reasoncode.InvalidMinLengthOrValue},
		{"string.min_len", reasoncode.InvalidMinLengthOrValue},
		{"repeated.min_items", reasoncode.InvalidMinLengthOrValue},
		{"int64.lte", reasoncode.InvalidMaxLengthOrValue},
		{"string.max_len", reasoncode.InvalidMaxLengthOrValue},
		{"string.len", reasoncode.InvalidMaxLengthOrValue},
		{"string.email", reasoncode.InvalidFormatEmail},
		{"string.uuid", reasoncode.InvalidFormatUUID},
		{"string.pattern", reasoncode.InvalidFormatRegex},
		{"string.uri_ref", reasoncode.InvalidFormatURL},
		{"enum.defined_only", reasoncode.InvalidEnumValue},
		{"enum.in", reasoncode.InvalidEnumValue},
	}
	for _, tt := range tests {
		t.Run(tt.ruleID, func(t *testing.T) {
			require.Equal(t, tt.want, r.Resolve(tt.ruleID))
		})
	}
}

func TestResolver_Resolve_UnknownAndAmbiguous(t *testing.T) {
	r := reasoncode.NewResolver(nil)

	// "const"/"in" are enum codes only when type-qualified as enum; the scalar
	// variants, the "required" rule (handled by ResolveViolation), the bare range
	// suffix and unmapped IDs all resolve to Unknown.
	for _, ruleID := range []string{"string.const", "int64.in", "required", "int64.gte_lte", "no.such.rule"} {
		t.Run(ruleID, func(t *testing.T) {
			require.Equal(t, reasoncode.Unknown, r.Resolve(ruleID))
		})
	}

	require.Equal(t, "", r.Resolve(""))
}

func TestResolver_Resolve_Catalog(t *testing.T) {
	r := reasoncode.NewResolver(map[string]string{"acme.string.thing": "INVALID_THING"})

	require.Equal(t, "INVALID_THING", r.Resolve("acme.string.thing"))
	require.Equal(t, reasoncode.InvalidMinLengthOrValue, r.Resolve("int64.gte"))
	require.Equal(t, reasoncode.Unknown, r.Resolve("acme.string.missing"))
}

func TestResolver_Resolve_CatalogPrefixGuard(t *testing.T) {
	catalog := map[string]string{"acme.up.range": "CUSTOM_RANGE"}

	t.Run("without_prefix_falls_through_to_standard", func(t *testing.T) {
		// "acme.up.gte" ends in a standard suffix and is mapped as such.
		require.Equal(t, reasoncode.InvalidMinLengthOrValue, reasoncode.NewResolver(catalog).Resolve("acme.up.gte"))
	})

	t.Run("with_prefix_uncatalogued_is_unknown", func(t *testing.T) {
		r := reasoncode.NewResolver(catalog, "acme.")
		require.Equal(t, reasoncode.Unknown, r.Resolve("acme.up.gte"))
		require.Equal(t, "CUSTOM_RANGE", r.Resolve("acme.up.range"))
		require.Equal(t, reasoncode.InvalidMinLengthOrValue, r.Resolve("int64.gte"))
	})
}

func TestResolver_ResolveViolation_Required(t *testing.T) {
	r := reasoncode.NewResolver(nil)

	require.Equal(t, "USER_NAME_REQUIRED", r.ResolveViolation(fakeViolation{ruleID: "required", fieldName: "userName"}))
	require.Equal(t, "REQUIRED", r.ResolveViolation(fakeViolation{ruleID: "required"}))
}

func TestResolver_ResolveViolation_Range(t *testing.T) {
	r := reasoncode.NewResolver(nil)

	tests := []struct {
		name string
		v    fakeViolation
		want string
	}{
		{
			"below_inclusive_min",
			fakeViolation{ruleID: "int64.gte_lte", value: 0, hasValue: true, lower: 1, inclusive: true, hasLower: true},
			reasoncode.InvalidMinLengthOrValue,
		},
		{
			"above_max",
			fakeViolation{ruleID: "int64.gte_lte", value: 1001, hasValue: true, lower: 1, inclusive: true, hasLower: true},
			reasoncode.InvalidMaxLengthOrValue,
		},
		{
			"at_exclusive_lower_is_min",
			fakeViolation{ruleID: "int64.gt_lt", value: 1, hasValue: true, lower: 1, inclusive: false, hasLower: true},
			reasoncode.InvalidMinLengthOrValue,
		},
		{
			"missing_value_falls_through_to_unknown",
			fakeViolation{ruleID: "int64.gte_lte", lower: 1, inclusive: true, hasLower: true},
			reasoncode.Unknown,
		},
		{
			"missing_bound_falls_through_to_unknown",
			fakeViolation{ruleID: "int64.gte_lte", value: 0, hasValue: true},
			reasoncode.Unknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, r.ResolveViolation(tt.v))
		})
	}
}

func TestResolver_ResolveViolation_FallsBackToResolve(t *testing.T) {
	r := reasoncode.NewResolver(map[string]string{"acme.string.thing": "INVALID_THING"}, "acme.")

	require.Equal(t, reasoncode.InvalidMinLengthOrValue, r.ResolveViolation(fakeViolation{ruleID: "int64.gte"}))
	require.Equal(t, "INVALID_THING", r.ResolveViolation(fakeViolation{ruleID: "acme.string.thing"}))
	require.Equal(t, reasoncode.Unknown, r.ResolveViolation(fakeViolation{ruleID: "acme.string.missing"}))
	require.Equal(t, "", r.ResolveViolation(fakeViolation{ruleID: ""}))
}
