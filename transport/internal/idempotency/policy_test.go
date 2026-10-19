// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package idempotency_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/transport/internal/idempotency"
)

func TestPolicyMandatory(t *testing.T) {
	t.Parallel()

	require.False(t, idempotency.PolicyDefault.Mandatory(false))
	require.True(t, idempotency.PolicyDefault.Mandatory(true))
	require.True(t, idempotency.PolicyMandatory.Mandatory(false))
	require.False(t, idempotency.PolicyOptional.Mandatory(true))
	require.False(t, idempotency.PolicySkip.Mandatory(true))
}

func TestOpaqueKeyValidator(t *testing.T) {
	t.Parallel()

	v := idempotency.OpaqueKeyValidator(8)
	for _, ok := range []string{"a", "req-1", "A_b.C~!#", "12345678"} {
		require.NoError(t, v(ok), ok)
	}
	for _, bad := range []string{"", "123456789", "has space", "tab\t", "κλειδί", "nul\x00", "del\x7f"} {
		require.ErrorIs(t, v(bad), idempotency.ErrInvalidFormat, "%q", bad)
	}
	require.NoError(t, idempotency.OpaqueKeyValidator(0)(strings.Repeat("k", idempotency.DefaultMaxKeyLength)))
	require.ErrorIs(t, idempotency.OpaqueKeyValidator(-1)(strings.Repeat("k", idempotency.DefaultMaxKeyLength+1)),
		idempotency.ErrInvalidFormat)
}

func TestPatternKeyValidator(t *testing.T) {
	t.Parallel()

	v := idempotency.PatternKeyValidator(regexp.MustCompile(`req-[0-9]+`), 10)
	require.NoError(t, v("req-42"))
	for _, bad := range []string{"", "xreq-42", "req-42x", "req-", "req-1234567"} {
		require.ErrorIs(t, v(bad), idempotency.ErrInvalidFormat, "%q", bad)
	}

	// Leftmost-first alternation would stop at "a"; the key must match whole.
	alt := idempotency.PatternKeyValidator(regexp.MustCompile(`a|ab`), 0)
	require.NoError(t, alt("ab"))
	require.NoError(t, alt("a"))
	require.ErrorIs(t, alt("abc"), idempotency.ErrInvalidFormat)

	anchored := idempotency.PatternKeyValidator(regexp.MustCompile(`^(?i)k[0-9]$`), 0)
	require.NoError(t, anchored("K7"), "flags and anchors in the pattern are kept")
}

func TestVerbMethodPolicy(t *testing.T) {
	t.Parallel()

	const p = idempotency.PolicyMandatory
	tests := map[string]idempotency.Policy{
		"/pkg.v1.Svc/CreateOrder":       p,
		"/pkg.v1.Svc/BatchCreateOrders": p,
		"/pkg.v1.Svc/CancelOrder":       p,
		"/pkg.v1.Svc/Getaway":           p, // "Get" is not a whole word here
		"/pkg.v1.Svc/GetOrder":          idempotency.PolicySkip,
		"/pkg.v1.Svc/ListOrders":        idempotency.PolicySkip,
		"/pkg.v1.Svc/SearchOrders":      idempotency.PolicySkip,
		"/pkg.v1.Svc/UpdateOrder":       idempotency.PolicySkip,
		"/pkg.v1.Svc/DeleteOrder":       idempotency.PolicySkip,
		"/pkg.v1.Svc/BatchGetOrders":    idempotency.PolicySkip,
		"/pkg.v1.Svc/BatchUpdateOrders": idempotency.PolicySkip,
		"/pkg.v1.Svc/BatchDeleteOrders": idempotency.PolicySkip,
		"/pkg.v1.Svc/Get":               idempotency.PolicySkip,
		"CreateOrder":                   p,
		"ListOrders":                    idempotency.PolicySkip,
	}
	for method, want := range tests {
		require.Equal(t, want, idempotency.VerbMethodPolicy(method, p, idempotency.DefaultSkipVerbs), method)
	}

	// Custom verbs replace the defaults.
	custom := []string{"Fetch", "Get"}
	require.Equal(t, idempotency.PolicySkip, idempotency.VerbMethodPolicy("/pkg.v1.Svc/FetchOrder", p, custom))
	require.Equal(t, p, idempotency.VerbMethodPolicy("/pkg.v1.Svc/ListOrders", p, custom))
	require.Equal(t, p, idempotency.VerbMethodPolicy("/pkg.v1.Svc/GetOrder", p, nil), "no skip verbs: every method gets p")

	// An empty verb prefixes every name; it must not skip every method.
	require.Equal(t, p, idempotency.VerbMethodPolicy("/pkg.v1.Svc/CreateOrder", p, []string{"Get", ""}))
	require.Equal(t, p, idempotency.VerbMethodPolicy("CreateOrder", p, []string{""}))
	require.Equal(t, idempotency.PolicySkip, idempotency.VerbMethodPolicy("/pkg.v1.Svc/GetOrder", p, []string{"", "Get"}))
}
