// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package middlewareconfig_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	coreerrs "github.com/altessa-s/go-atlas-core/errors"
	middlewareconfig "github.com/altessa-s/go-atlas/config/middleware"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func fieldErrors(t *testing.T, err error) validation.Errors {
	t.Helper()
	verrs, ok := errors.AsType[validation.Errors](err)
	require.True(t, ok, "want validation.Errors, got %v", err)
	return verrs
}

func TestValidateGeoACLRule(t *testing.T) {
	t.Parallel()

	require.NoError(t, middlewareconfig.ValidateGeoACLRule("not a rule"), "foreign types are ignored")

	valid := middlewareconfig.GeoACLRule{
		Patterns:        []string{"^/api/.*$"},
		AllowContinents: []string{"EU"},
		DenyContinents:  []string{"AN"},
		AllowCountries:  []string{"DE"},
		DenyCountries:   []string{"RU"},
		AllowRegions:    []string{"US-CA"},
		DenyRegions:     []string{"GB-ENG"},
	}
	require.NoError(t, middlewareconfig.ValidateGeoACLRule(valid))

	invalid := middlewareconfig.GeoACLRule{
		Patterns:        []string{"(unclosed"},
		AllowContinents: []string{"XX"},
		DenyContinents:  []string{"eu"},
		AllowCountries:  []string{"DEU"},
		DenyCountries:   []string{"r1"},
		AllowRegions:    []string{"USCA"},
		DenyRegions:     []string{"us-ca"},
	}
	verrs := fieldErrors(t, middlewareconfig.ValidateGeoACLRule(invalid))
	for _, f := range []string{
		"Patterns", "AllowContinents", "DenyContinents", "AllowCountries", "DenyCountries", "AllowRegions", "DenyRegions",
	} {
		require.Contains(t, verrs, f)
	}
}

func TestValidateIPACLRule(t *testing.T) {
	t.Parallel()

	require.NoError(t, middlewareconfig.ValidateIPACLRule(42), "foreign types are ignored")
	require.NoError(t, middlewareconfig.ValidateIPACLRule(middlewareconfig.IPACLRule{
		Patterns:  []string{"^/admin"},
		Allowlist: []string{"10.0.0.0/8", "2001:db8::/32"},
		Denylist:  []string{"192.168.1.1/32"},
	}))

	verrs := fieldErrors(t, middlewareconfig.ValidateIPACLRule(middlewareconfig.IPACLRule{
		Patterns:  []string{""},
		Allowlist: []string{"not-an-ip"},
		Denylist:  []string{"300.0.0.0/8"},
	}))
	for _, f := range []string{"Patterns", "Allowlist", "Denylist"} {
		require.Contains(t, verrs, f)
	}
}

func TestValidateIP(t *testing.T) {
	t.Parallel()

	require.NoError(t, middlewareconfig.ValidateIP("10.1.2.0/24"))
	require.ErrorIs(t, middlewareconfig.ValidateIP("bogus"), coreerrs.ErrInvalidArgument)
	require.ErrorIs(t, middlewareconfig.ValidateIP(123), coreerrs.ErrInvalidArgument)
}

func TestFallbackBehaviorRule(t *testing.T) {
	t.Parallel()

	require.Equal(t, []middlewareconfig.FallbackBehavior{
		middlewareconfig.FallbackBehaviorAllow, middlewareconfig.FallbackBehaviorDeny, middlewareconfig.FallbackBehaviorError,
	}, middlewareconfig.AllFallbackBehaviors())

	rule := middlewareconfig.FallbackBehaviorRule()
	require.NoError(t, rule.Validate(middlewareconfig.FallbackBehaviorDeny))
	require.Error(t, rule.Validate(middlewareconfig.FallbackBehavior("ignore")))
}

type idemCfg struct {
	Format            middlewareconfig.IdempotencyKeyFormat
	Pattern           string
	MaxLen            int
	Mandatory         []string
	MandatoryPatterns []string
}

func (c *idemCfg) Validate() error {
	return validation.ValidateStruct(c, middlewareconfig.IdempotencyKeyRules(
		&c.Format, &c.Pattern, &c.MaxLen, &c.Mandatory, &c.MandatoryPatterns)...)
}

func TestIdempotencyKeyRules(t *testing.T) {
	t.Parallel()

	require.Len(t, middlewareconfig.AllIdempotencyKeyFormats(), 3)
	require.NoError(t, middlewareconfig.IdempotencyKeyFormatRule().Validate(middlewareconfig.IdempotencyKeyFormatOpaque))

	require.NoError(t, (&idemCfg{}).Validate(), "empty format uses the default")
	require.NoError(t, (&idemCfg{
		Format: middlewareconfig.IdempotencyKeyFormatPattern, Pattern: "^[a-z]+$", MaxLen: 64,
		Mandatory: []string{"POST /orders"}, MandatoryPatterns: []string{"^/pay"},
	}).Validate())

	tests := []struct {
		name  string
		cfg   idemCfg
		field string
	}{
		{"unknown format", idemCfg{Format: "ulid"}, "Format"},
		{"pattern format without pattern", idemCfg{Format: middlewareconfig.IdempotencyKeyFormatPattern}, "Pattern"},
		{"invalid pattern", idemCfg{Format: middlewareconfig.IdempotencyKeyFormatPattern, Pattern: "("}, "Pattern"},
		{"negative max length", idemCfg{MaxLen: -1}, "MaxLen"},
		{"empty mandatory entry", idemCfg{Mandatory: []string{""}}, "Mandatory"},
		{"invalid mandatory pattern", idemCfg{MandatoryPatterns: []string{"[a-"}}, "MandatoryPatterns"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Contains(t, fieldErrors(t, tc.cfg.Validate()), tc.field)
		})
	}
}
