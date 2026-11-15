// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package geoacl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A GeoIP database answers a loopback or private address with an empty
// GeoInfo and no error. Such a caller is on no list: a sanctions deny list
// must not turn away kubelet probes and in-cluster callers, while an allow
// list still cannot be satisfied by a location nobody could determine.
func TestEvaluate_UnknownLocationIsOnNoList(t *testing.T) {
	t.Parallel()
	const endpoint = "/api/orders"

	sanctions := &AccessRule{DenyCountries: []string{"KP", "IR"}}
	mixed := &AccessRule{DenyCountries: []string{"KP"}, AllowContinents: []string{"EU"}}

	tests := []struct {
		name   string
		policy Policy
		rule   *AccessRule
		geo    GeoInfo
		want   bool
	}{
		{"deny list under PolicyAllow lets an unknown location through", PolicyAllow, sanctions, GeoInfo{}, true},
		{"deny list under PolicyDeny keeps the registry default", PolicyDeny, sanctions, GeoInfo{}, false},
		{"deny list still denies a listed country", PolicyAllow, sanctions, GeoInfo{CountryCode: "KP"}, false},
		{"allow list denies an unknown location under PolicyAllow", PolicyAllow, &AccessRule{AllowCountries: []string{"US"}}, GeoInfo{}, false},
		{"allow list denies an unknown location under PolicyDeny", PolicyDeny, &AccessRule{AllowCountries: []string{"US"}}, GeoInfo{}, false},
		{"allow and deny lists deny an unknown location", PolicyAllow, mixed, GeoInfo{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reg := NewRegistry(tc.policy)
			reg.SetDefault(tc.rule)

			got, err := reg.Evaluate(t.Context(), &mockResolver{geo: tc.geo}, testIP, endpoint)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
