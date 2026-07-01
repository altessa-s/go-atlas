// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas/config"
)

func TestWebhookSignatureValidate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		cfg     config.WebhookSignature
		wantErr bool
	}{
		{"valid_github", config.WebhookSignature{Scheme: config.WebhookSchemeGitHub, Secret: "s"}, false},
		{"valid_stripe", config.WebhookSignature{Scheme: config.WebhookSchemeStripe, Secret: "s", Tolerance: time.Minute}, false},
		{"missing_scheme", config.WebhookSignature{Secret: "s"}, true},
		{"unknown_scheme", config.WebhookSignature{Scheme: "nope", Secret: "s"}, true},
		{"missing_secret", config.WebhookSignature{Scheme: config.WebhookSchemeGitHub}, true},
		{"negative_tolerance", config.WebhookSignature{Scheme: config.WebhookSchemeGitHub, Secret: "s", Tolerance: -time.Second}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.cfg.Validate()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
