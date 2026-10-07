// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package webhookconfig_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	webhookconfig "github.com/altessa-s/go-atlas/config/webhook"
)

func TestWebhookSignatureValidate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		cfg     webhookconfig.Signature
		wantErr bool
	}{
		{"valid_github", webhookconfig.Signature{Scheme: webhookconfig.SchemeGitHub, Secret: "s"}, false},
		{"valid_stripe", webhookconfig.Signature{Scheme: webhookconfig.SchemeStripe, Secret: "s", Tolerance: time.Minute}, false},
		{"missing_scheme", webhookconfig.Signature{Secret: "s"}, true},
		{"unknown_scheme", webhookconfig.Signature{Scheme: "nope", Secret: "s"}, true},
		{"missing_secret", webhookconfig.Signature{Scheme: webhookconfig.SchemeGitHub}, true},
		{"negative_tolerance", webhookconfig.Signature{Scheme: webhookconfig.SchemeGitHub, Secret: "s", Tolerance: -time.Second}, true},
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
