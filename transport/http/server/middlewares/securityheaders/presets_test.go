// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package securityheaders

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func headersFor(t *testing.T, opts ...Option) http.Header {
	t.Helper()
	handler := New(opts...).Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/test", nil))
	return rec.Header()
}

// TestCrossOriginHeaders_OptIn pins that COOP/COEP/CORP are emitted only when
// configured and absent by default.
func TestCrossOriginHeaders_OptIn(t *testing.T) {
	t.Parallel()

	off := headersFor(t)
	require.Empty(t, off.Get(HeaderCrossOriginOpenerPol))
	require.Empty(t, off.Get(HeaderCrossOriginEmbedderPol))
	require.Empty(t, off.Get(HeaderCrossOriginResourcePol))

	on := headersFor(t,
		WithCrossOriginOpenerPolicy(COOPSameOrigin),
		WithCrossOriginEmbedderPolicy(COEPRequireCorp),
		WithCrossOriginResourcePolicy(CORPSameOrigin),
	)
	require.Equal(t, "same-origin", on.Get(HeaderCrossOriginOpenerPol))
	require.Equal(t, "require-corp", on.Get(HeaderCrossOriginEmbedderPol))
	require.Equal(t, "same-origin", on.Get(HeaderCrossOriginResourcePol))
}

// TestRecommendedOptions verifies the safe baseline: it sets the non-breaking
// headers plus a Permissions-Policy, and never sets CSP/HSTS/COOP/COEP/CORP.
func TestRecommendedOptions(t *testing.T) {
	t.Parallel()

	h := headersFor(t, RecommendedOptions()...)

	require.Equal(t, string(FrameOptionsDeny), h.Get(HeaderXFrameOptions))
	require.Equal(t, string(ReferrerPolicyStrictOriginWhenCrossOrigin), h.Get(HeaderReferrerPolicy))
	require.Equal(t, "nosniff", h.Get(HeaderXContentTypeOptions))
	require.Equal(t, "0", h.Get(HeaderXXSSProtection))
	require.Equal(t, RecommendedPermissionsPolicy, h.Get(HeaderPermissionsPolicy))

	// Must NOT impose deployment-specific / breaking headers.
	require.Empty(t, h.Get(HeaderContentSecurityPolicy))
	require.Empty(t, h.Get(HeaderStrictTransportSec))
	require.Empty(t, h.Get(HeaderCrossOriginOpenerPol))
	require.Empty(t, h.Get(HeaderCrossOriginEmbedderPol))
	require.Empty(t, h.Get(HeaderCrossOriginResourcePol))
}

// TestStrictOptions verifies Strict adds COOP+CORP (same-origin) but still never
// sets the disruptive COEP, nor CSP/HSTS.
func TestStrictOptions(t *testing.T) {
	t.Parallel()

	h := headersFor(t, StrictOptions()...)

	require.Equal(t, "same-origin", h.Get(HeaderCrossOriginOpenerPol))
	require.Equal(t, "same-origin", h.Get(HeaderCrossOriginResourcePol))
	require.Equal(t, RecommendedPermissionsPolicy, h.Get(HeaderPermissionsPolicy))

	require.Empty(t, h.Get(HeaderCrossOriginEmbedderPol), "COEP must stay opt-in even in Strict")
	require.Empty(t, h.Get(HeaderContentSecurityPolicy))
	require.Empty(t, h.Get(HeaderStrictTransportSec))
}
