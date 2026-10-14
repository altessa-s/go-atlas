// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package limiterconfig_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	limiterconfig "github.com/altessa-s/go-atlas/config/limiter"
)

func TestNATSContention_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     limiterconfig.NATSContention
		wantErr bool
	}{
		{name: "defaults", cfg: limiterconfig.NATSContention{MaxCASAttempts: 64, MaxCASBackoff: 20 * time.Millisecond}},
		{name: "zero keeps storage defaults", cfg: limiterconfig.NATSContention{}},
		{name: "negative attempts", cfg: limiterconfig.NATSContention{MaxCASAttempts: -1}, wantErr: true},
		{name: "negative backoff", cfg: limiterconfig.NATSContention{MaxCASBackoff: -time.Millisecond}, wantErr: true},
	}
	for _, tc := range tests {
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
