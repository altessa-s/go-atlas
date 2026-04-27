// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package factory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResolveTtl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		override time.Duration
		cfgTtl   time.Duration
		want     time.Duration
	}{
		{"both zero falls back to provider default", 0, 0, 0},
		{"config supplies value", 0, 5 * time.Minute, 5 * time.Minute},
		{"override wins over config", 10 * time.Minute, 5 * time.Minute, 10 * time.Minute},
		{"override wins when config zero", 10 * time.Minute, 0, 10 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := &UniqBuilder{ttlOverride: tt.override}
			require.Equal(t, tt.want, b.resolveTtl(tt.cfgTtl))
		})
	}
}

func TestUseTtl_IgnoresNonPositive(t *testing.T) {
	t.Parallel()

	b := &UniqBuilder{}
	b.UseTtl(0).UseTtl(-1 * time.Second)
	require.Equal(t, time.Duration(0), b.ttlOverride,
		"UseTtl(0) and UseTtl(negative) must leave ttlOverride untouched")

	b.UseTtl(7 * time.Second)
	require.Equal(t, 7*time.Second, b.ttlOverride)
}
