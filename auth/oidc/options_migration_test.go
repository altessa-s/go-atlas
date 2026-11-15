// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import "testing"

func TestProviderOptionMigrationBaseline(t *testing.T) {
	t.Run("failure modes", func(t *testing.T) {
		got := defaultOptions()
		WithJWKSFailureMode(JWKSFailureMode("invalid"))(got)
		WithAudienceFailureMode(AudienceFailureMode("invalid"))(got)
		WithDiscoveryValidationMode(DiscoveryValidationMode("invalid"))(got)
		if got.jwksFailureMode != DefaultJWKSFailureMode || got.audienceFailureMode != DefaultAudienceFailureMode || got.discoveryValidationMode != DefaultDiscoveryValidationMode {
			t.Fatal("invalid mode changed a default")
		}
		WithJWKSFailureMode(JWKSFailureModeWarn)(got)
		WithAudienceFailureMode(AudienceFailureModeWarn)(got)
		WithDiscoveryValidationMode(DiscoveryValidationModeWarn)(got)
		if got.jwksFailureMode != JWKSFailureModeWarn || got.audienceFailureMode != AudienceFailureModeWarn || got.discoveryValidationMode != DiscoveryValidationModeWarn {
			t.Fatal("valid mode was not applied")
		}
		WithJWKSFailureMode(JWKSFailureMode("invalid"))(got)
		WithAudienceFailureMode(AudienceFailureMode("invalid"))(got)
		WithDiscoveryValidationMode(DiscoveryValidationMode("invalid"))(got)
		if got.jwksFailureMode != JWKSFailureModeWarn || got.audienceFailureMode != AudienceFailureModeWarn || got.discoveryValidationMode != DiscoveryValidationModeWarn {
			t.Fatal("invalid mode replaced a configured value")
		}
	})
	t.Run("introspection fail open", func(t *testing.T) {
		got := defaultOptions()
		WithIntrospectionFailOpen()(got)
		if !got.introspectionFailOpen {
			t.Fatal("fail-open option did not enable the flag")
		}
	})
	t.Run("default validation options", func(t *testing.T) {
		got := defaultOptions()
		WithDefaultValidationOptions()(got)
		if got.verifierOptions == nil {
			t.Fatal("empty call did not install an empty options value")
		}
		defer func() {
			if recover() == nil {
				t.Fatal("nil option did not panic")
			}
		}()
		WithDefaultValidationOptions(nil)(got)
	})
}
