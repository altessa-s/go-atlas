// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package grpcconfig_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	grpcconfig "github.com/altessa-s/go-atlas/config/grpc"
)

// Private client addresses are exempt from the geographic check only when the
// operator asks for it.
func TestGeoACLInterceptor_AllowPrivateAddresses(t *testing.T) {
	t.Parallel()

	def := grpcconfig.DefaultGeoACLInterceptor()
	require.False(t, def.AllowPrivateAddresses, "off by default")

	tests := []struct {
		name string
		yaml string
		want bool
	}{
		{"key absent", "enabled: true\n", false},
		{"false", "enabled: true\nallowPrivateAddresses: false\n", false},
		{"true", "enabled: true\nallowPrivateAddresses: true\n", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := grpcconfig.DefaultGeoACLInterceptor()
			dec := yaml.NewDecoder(strings.NewReader(tc.yaml))
			dec.KnownFields(true)
			require.NoError(t, dec.Decode(&cfg))

			require.Equal(t, tc.want, cfg.AllowPrivateAddresses)
			require.True(t, cfg.Enabled, "the inline base keys still decode")
			require.NoError(t, cfg.Validate())
		})
	}
}
