// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package httpconfig_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	httpconfig "github.com/altessa-s/go-atlas/config/http"
)

func TestCORSMiddlewareRejectsCredentialedWildcardOrigins(t *testing.T) {
	t.Parallel()

	for _, origin := range []string{"*", "https://app*.example.com"} {
		t.Run(origin, func(t *testing.T) {
			t.Parallel()
			cfg := httpconfig.DefaultCORSMiddleware()
			cfg.Enabled = true
			cfg.AllowedOrigins = []string{origin}
			require.NoError(t, cfg.Validate())
			cfg.AllowCredentials = true
			require.ErrorContains(t, cfg.Validate(), httpconfig.ErrCredentialedWildcardOrigin.Error())
			cfg.AllowedOrigins = []string{"https://app.example.com"}
			require.NoError(t, cfg.Validate())
		})
	}
}
