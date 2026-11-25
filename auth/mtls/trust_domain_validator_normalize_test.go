// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package mtls_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/auth/mtls"
)

// TrustDomainValidator normalizes its domains as WithTrustDomains does: blank
// entries pin nothing, and matching ignores case and surrounding space.
func TestTrustDomainValidatorNormalizes(t *testing.T) {
	t.Parallel()
	local := svidCert(t, "spiffe://example.org/x")

	for _, blank := range [][]string{{""}, {"  "}, {"", "\t"}} {
		require.ErrorIs(t, mtls.TrustDomainValidator(blank...)(t.Context(), local), mtls.ErrNoTrustDomainPolicy, "%q", blank)
	}

	v := mtls.TrustDomainValidator("", " Example.ORG ")
	require.NoError(t, v(t.Context(), local))
	require.ErrorIs(t, v(t.Context(), svidCert(t, "spiffe://foreign.example/x")), mtls.ErrUntrustedDomain)
}
