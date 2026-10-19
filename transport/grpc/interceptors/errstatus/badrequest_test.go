// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package errstatus

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/altessa-s/proto-gen-go/io/altessa/badrequest/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// requireViolation asserts err is InvalidArgument "<path>: <reason>" with
// exactly one altessa BadRequest violation on path and no other detail.
func requireViolation(t *testing.T, err error, path, reason string) {
	t.Helper()
	st, ok := status.FromError(err)
	require.True(t, ok, "err = %v, want a status", err)
	require.Equal(t, codes.InvalidArgument, st.Code())
	require.Equal(t, path+": "+reason, st.Message())
	require.Len(t, st.Details(), 1)
	br, ok := st.Details()[0].(*badrequestv1.BadRequest)
	require.True(t, ok, "detail = %T, want the altessa BadRequest", st.Details()[0])
	require.Len(t, br.GetFieldViolations(), 1)
	v := br.GetFieldViolations()[0]
	assert.Equal(t, path, v.GetFieldPath())
	assert.Equal(t, reason, v.GetMessage())
	assert.Nil(t, v.Code, "no machine code is set")
}

func TestBadRequest(t *testing.T) {
	t.Parallel()

	requireViolation(t, BadRequest("bot.filters.minPrice.currencyCode", "must be RUB").Err(), "bot.filters.minPrice.currencyCode", "must be RUB")
	requireViolation(t, FieldViolation("provider.baseUrl", "must be https"), "provider.baseUrl", "must be https")
}

func TestCamelPath(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{
		"":                              "",
		"name":                          "name",
		"base_url":                      "baseUrl",
		"attrs.law_code":                "attrs.lawCode",
		"licenseType.defaultValidity":   "licenseType.defaultValidity",
		"license_type.default_validity": "licenseType.defaultValidity",
		"key_last4":                     "keyLast4",
		"digest_schedule.times[0]":      "digestSchedule.times[0]",
		"prices[2].cache_read":          "prices[2].cacheRead",
		"Name.Sub_field":                "name.subField",
		"a..b":                          "a..b",
		"όνομα_πεδίου":                  "όνομαΠεδίου",
	} {
		got := CamelPath(in)
		require.Equal(t, want, got, in)
		require.Equal(t, got, CamelPath(got), "CamelPath must be idempotent for %q", in)
	}
}

func TestPrefixed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		prefix, field, want string
	}{
		{"provider", "max_tokens", "provider.maxTokens"},
		{"provider", "base_url", "provider.baseUrl"},
		{"referenceItem", "attrs.law_code", "referenceItem.attrs.lawCode"},
		{"rateSettings", "sources[1].enabled", "rateSettings.sources[1].enabled"},
		{"", "attrs.law_code", "attrs.lawCode"},
	}
	for _, tc := range tests {
		require.Equal(t, tc.want, Prefixed(tc.prefix)(tc.field), "%q under %q", tc.field, tc.prefix)
	}
}
