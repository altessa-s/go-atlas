// Copyright 2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package oidc

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func newAudienceTestProvider(mode AudienceFailureMode) *Provider {
	return &Provider{
		opts:   &options{audienceFailureMode: mode},
		logger: slog.New(slog.DiscardHandler),
	}
}

func TestProvider_checkAudienceConfigured(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mode     AudienceFailureMode
		audience []string
		wantErr  bool
	}{
		{name: "enforce without audience rejects", mode: AudienceFailureModeEnforce, wantErr: true},
		{name: "enforce with audience passes", mode: AudienceFailureModeEnforce, audience: []string{"svc"}},
		{name: "warn without audience passes", mode: AudienceFailureModeWarn},
		{name: "disabled without audience passes", mode: AudienceFailureModeDisabled},
		{name: "default (zero mode) without audience rejects", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := newAudienceTestProvider(tt.mode)
			err := p.checkAudienceConfigured(&verifierOptions{audience: tt.audience})

			if tt.wantErr {
				require.ErrorIs(t, err, ErrAudienceNotConfigured)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestWithAudienceFailureMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode AudienceFailureMode
		want AudienceFailureMode
	}{
		{name: "enforce", mode: AudienceFailureModeEnforce, want: AudienceFailureModeEnforce},
		{name: "warn", mode: AudienceFailureModeWarn, want: AudienceFailureModeWarn},
		{name: "disabled", mode: AudienceFailureModeDisabled, want: AudienceFailureModeDisabled},
		{name: "unknown keeps default", mode: "bogus", want: DefaultAudienceFailureMode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			o := &options{audienceFailureMode: DefaultAudienceFailureMode}
			WithAudienceFailureMode(tt.mode)(o)
			require.Equal(t, tt.want, o.audienceFailureMode)
		})
	}
}
